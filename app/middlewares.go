package app

import (
	"context"
	xerrors "errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi/v5"
	"github.com/mariusor/go-littr/internal/log"
)

func (h handler) LoadAuthorMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handle := chi.URLParam(r, "handle")
		if handle == "" {
			h.ErrorHandler(errors.NotValidf("missing account handle")).ServeHTTP(w, r)
			return
		}
		var authors AccountCollection
		if handle == selfName {
			self := Account{}
			self.FromActivityPub(h.storage.fedbox.Service())
			authors = AccountCollection{self}
		} else {
			var err error
			repo := ContextRepository(r.Context())
			ctx, _ := context.WithTimeout(r.Context(), time.Second)
			ctx = context.WithValue(ctx, LoggedAccountCtxtKey, ContextAccount(r.Context()))

			instance := repo.fedbox.Service().GetLink()
			if strings.Contains(handle, "@") {
				var inst string
				handle, inst = splitRemoteHandle(handle)
				instance = pub.IRI(fmt.Sprintf("https://%s", inst))
			}
			authors, err = repo.accountsFromRemote(ctx, instance, FilterAccountByHandle(handle))
			if err != nil {
				h.ErrorHandler(err).ServeHTTP(w, r)
				return
			}
		}
		if len(authors) == 0 {
			h.ErrorHandler(errors.NotFoundf("Account %q", chi.URLParam(r, "handle"))).ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), AuthorCtxtKey, authors)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ctxtErr(next http.Handler, w http.ResponseWriter, r *http.Request, err error) {
	status := errors.HttpStatus(err)
	ctx := context.WithValue(r.Context(), ModelCtxtKey, &errorModel{
		Status:     status,
		Title:      fmt.Sprintf("Error %d", status),
		StatusText: http.StatusText(status),
		Errors:     []error{err},
	})
	next.ServeHTTP(w, r.WithContext(ctx))
}

type CollectionLoadFn func(pub.CollectionInterface) error

func LoadMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repo := ContextRepository(r.Context())
		searches := ContextLoads(r.Context())
		deps := ContextDependentLoads(r.Context())

		c, err := repo.LoadSearches(r.Context(), searches, *deps)
		if err != nil {
			ctxtErr(next, w, r, errors.NotFoundf(strings.TrimLeft(r.URL.Path, "/")))
			return
		}
		ctx := context.WithValue(r.Context(), CursorCtxtKey, &c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestHandleSearches(r *http.Request) pub.ItemCollection {
	authors := ContextAuthors(r.Context())
	if len(authors) == 0 {
		return nil
	}

	actors := make(pub.ItemCollection, 0)
	for _, author := range authors {
		if author.pub == nil {
			continue
		}
		actors = append(actors, author.pub)
	}
	return actors
}

func loggedAccountSearches(collections ...LoadFn) func(http.Handler) http.Handler {
	return SearchInCollectionsMw(getLoggedActorFn, collections...)
}

func serviceSearches(collections ...LoadFn) func(http.Handler) http.Handler {
	return SearchInCollectionsMw(getServiceFn, collections...)
}

func OperatorSearches(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchIn := ContextLoads(r.Context())
		repo := ContextRepository(r.Context())

		base := baseIRI(repo.app.pub.GetLink())
		ff := new(Filters)
		ff.Type = ActivityTypesFilter(pub.FollowType)
		opSearch := RemoteLoad{
			actor:   repo.app.pub,
			loadFn:  outbox,
			filters: []*Filters{ff},
		}
		searches, ok := searchIn[base]
		if !ok {
			searches = make([]RemoteLoad, 0)
		}
		searches = append(searches, opSearch)
		searchIn[base] = searches

		next.ServeHTTP(w, r)
	})
}

func applicationSearches(collections ...LoadFn) func(http.Handler) http.Handler {
	return SearchInCollectionsMw(getApplicationFn, collections...)
}

func getServiceFn(r *http.Request) pub.ItemCollection {
	return pub.ItemCollection{ContextRepository(r.Context()).fedbox.Service()}
}

func getApplicationFn(r *http.Request) pub.ItemCollection {
	if a := ContextRepository(r.Context()).app; a != nil {
		return pub.ItemCollection{a.pub}
	}
	return pub.ItemCollection{}
}

func getLoggedActorFn(r *http.Request) pub.ItemCollection {
	if logged := ContextAccount(r.Context()); logged.IsLogged() {
		return pub.ItemCollection{logged.pub}
	}
	return nil
}

func SearchInCollectionsMw(getActorsFn func(r *http.Request) pub.ItemCollection, collections ...LoadFn) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ff := ContextActivityFilters(r.Context())
			searchIn := ContextLoads(r.Context())
			storeSearches := false
			if searchIn == nil {
				searchIn = RemoteLoads{}
				storeSearches = true
			}

			actors := getActorsFn(r)
			if actors == nil {
				next.ServeHTTP(w, r)
				return
			}
			for _, current := range actors {
				if current == nil {
					continue
				}
				base := baseIRI(current.GetLink())
				for _, collectionFn := range collections {
					searchIn[base] = append(
						searchIn[base],
						RemoteLoad{actor: current, loadFn: collectionFn, filters: ff},
					)
				}
			}
			if storeSearches {
				rtx := context.WithValue(r.Context(), LoadsCtxtKey, searchIn)
				next.ServeHTTP(w, r.WithContext(rtx))
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}
}

func LoadSingleItemMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		deps := ContextDependentLoads(r.Context())
		repo := ContextRepository(r.Context())
		item := ContextItem(r.Context())
		if !item.IsValid() {
			ctxtErr(next, w, r, errors.NotFoundf(strings.TrimLeft(r.URL.Path, "/")))
			return
		}

		items := ItemCollection{*item}
		if deps.Replies {
			if comments, err := repo.loadItemsReplies(r.Context(), items...); err == nil {
				items = append(items, comments...)
			}
		}
		if deps.Authors {
			if items, err = repo.loadItemsAuthors(r.Context(), items...); err != nil {
				repo.errFn()("unable to load item authors")
			}
		}
		if deps.Votes {
			if items, err = repo.loadItemsVotes(r.Context(), items...); err != nil {
				repo.errFn()("unable to load item votes")
			}
		}
		c := Cursor{
			items: make(RenderableList),
		}
		for k := range items {
			c.items.Append(Renderable(&items[k]))
		}

		rtx := context.WithValue(r.Context(), CursorCtxtKey, &c)
		next.ServeHTTP(w, r.WithContext(rtx))
	})

}

func SingleItemModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := ContextContentModel(r.Context())
		if m == nil {
			next.ServeHTTP(w, r)
			return
		}
		item := ContextItem(r.Context())

		m.Title = "Replies to item"
		if item.SubmittedBy != nil {
			m.Title = fmt.Sprintf("Replies to %s item", genitive(item.SubmittedBy.Handle))
		}
		if len(item.Title) > 0 {
			m.Title = fmt.Sprintf("%s: %s", m.Title, item.Title)
		}
		ctx := context.WithValue(r.Context(), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoadSingleObjectMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repo := ContextRepository(r.Context())
		searchIn := ContextLoads(r.Context())

		var item Item
		err := LoadFromSearches(r.Context(), repo, searchIn, func(ctx context.Context, c pub.CollectionInterface, f *Filters) error {
			for _, it := range c.Collection() {
				err := pub.OnActivity(it, func(act *pub.Activity) error {
					if act.Object.IsLink() {
						ob, err := repo.fedbox.Object(ctx, act.Object.GetLink())
						if err != nil {
							repo.errFn(log.Ctx{"iri": act.Object.GetLink()})("unable to load item")
							return err
						}
						act.Object = ob
					}
					if err := item.FromActivityPub(act.Object); err != nil {
						return err
					}
					return nil
				})
				if err != nil {
					return err
				}
				if item.IsValid() {
					return StopLoad{}
				}
			}
			return nil
		})
		if (err != nil && !xerrors.Is(err, StopLoad{})) || !item.IsValid() {
			ctx := log.Ctx{}
			if err != nil {
				ctx["err"] = err.Error()
			}
			repo.errFn(ctx)("item not found")
			ctxtErr(next, w, r, errors.NotFoundf(strings.TrimLeft(r.URL.Path, "/")))
			return
		}
		rtx := context.WithValue(r.Context(), ContentCtxtKey, &item)
		next.ServeHTTP(w, r.WithContext(rtx))
	})
}

func ModelMw(m Model) Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authors := ContextAuthors(r.Context())
			if len(authors) > 0 {

			}
			ctx := context.WithValue(r.Context(), ModelCtxtKey, m)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ListingModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := new(listingModel)
		m.sortFn = ByDate
		ctx := context.WithValue(r.Context(), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AccountListingModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := new(listingModel)
		m.sortFn = ByDate
		m.tpl = "user"
		m.ShowText = true
		authors := ContextAuthors(r.Context())
		if len(authors) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		auth := authors[0]
		if len(authors) > 0 && auth.IsValid() {
			m.User = &auth
		}
		m.Title = fmt.Sprintf("%s submissions", genitive(auth.Handle))
		ctx := context.WithValue(r.Context(), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ContentModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		m := ContextContentModel(ctx)
		if m == nil {
			m = new(contentModel)
			m.Content = new(Item)
		}
		m.Content = new(Item)
		m.Message.Label = "Reply:"
		m.Message.Back = "/"
		m.Message.SubmitLabel = htmlf("Reply %s", icon("reply", "h-mirror"))
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, ModelCtxtKey, m)))
	})
}

func htmlf(s string, p ...interface{}) template.HTML {
	return template.HTML(fmt.Sprintf(s, p...))
}

func EditContentModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		m := ContextContentModel(ctx)
		if m == nil {
			m = new(contentModel)
			m.Content = new(Item)
		}
		m.Title = "Edit item"
		m.Message.Editable = true
		m.Message.Label = "Edit:"
		m.Message.Back = "/"
		m.Message.SubmitLabel = htmlf("%s Save", icon("edit"))
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, ModelCtxtKey, m)))
	})
}

func AddModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		m := ContextContentModel(ctx)
		if m == nil {
			m = new(contentModel)
			m.Content = new(Item)
		}
		m.tpl = "new"
		m.Message.ShowTitle = true
		m.Title = "Add new submission"
		m.Message.Editable = true
		m.Message.Label = "Add new submission:"
		m.Message.Back = "/"
		m.Message.SubmitLabel = htmlf("%s Submit", icon("reply", "h-mirror", "v-mirror"))
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, ModelCtxtKey, m)))
	})
}

func reportModelFromCtx(ctx context.Context) *moderationModel {
	if _, ok := ContextModel(ctx).(*errorModel); ok {
		return nil
	}
	m := ContextModerationModel(ctx)
	if m == nil {
		m = new(moderationModel)
		m.Content = new(ModerationOp)
		m.Content.pub = &pub.Flag{Type: pub.FlagType}
	}
	m.Message.Editable = false
	m.Message.SubmitLabel = htmlf("%s Report", icon("flag"))
	m.Message.Label = "Please add your reason for reporting:"
	m.Message.Back = "/"

	return m
}

func ReportContentModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		m := reportModelFromCtx(ctx)
		if m == nil {
			next.ServeHTTP(w, r)
		}
		if hash := HashFromString(chi.URLParam(r, "hash")); hash.IsValid() {
			m.Hash = hash
		}

		m.Title = "Report item"
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, ModelCtxtKey, m)))
	})
}

func ReportAccountModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		m := reportModelFromCtx(ctx)
		if m == nil {
			next.ServeHTTP(w, r)
		}
		authors := ContextAuthors(ctx)
		if len(authors) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		auth := authors[0]
		m.Content.Object = &auth
		m.Hash = auth.Hash
		m.Title = fmt.Sprintf("Report %s", auth.Handle)
		m.Message.Label = fmt.Sprintf("Report %s:", auth.Handle)
		m.Title = "Report account"
		m.Message.Back = PermaLink(&auth)
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, ModelCtxtKey, m)))
	})
}

func blockModelFromCtx(ctx context.Context) *moderationModel {
	if _, ok := ContextModel(ctx).(*errorModel); ok {
		return nil
	}
	m := ContextModerationModel(ctx)
	if m == nil {
		m = new(moderationModel)
		m.Content = new(ModerationOp)
		m.Content.pub = &pub.Block{Type: pub.BlockType}
	}
	m.Message.Editable = false
	m.Title = fmt.Sprintf("Block item")
	m.Message.Label = fmt.Sprintf("Block item:")
	m.Message.SubmitLabel = htmlf("%s Block", icon("block"))
	m.Message.Label = "Please add your reason for blocking:"
	m.Message.Back = "/"

	return m
}

func BlockAccountModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		m := blockModelFromCtx(ctx)
		if m == nil {
			next.ServeHTTP(w, r)
		}
		authors := ContextAuthors(ctx)
		if len(authors) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		auth := authors[0]
		m.Content.Object = &auth
		m.Title = fmt.Sprintf("Block %s", auth.Handle)
		m.Message.Label = fmt.Sprintf("Block %s:", auth.Handle)
		m.Message.Back = PermaLink(&auth)
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, ModelCtxtKey, m)))
	})
}

func BlockContentModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if c := ContextCursor(ctx); c == nil {
			next.ServeHTTP(w, r)
			return
		}
		m := blockModelFromCtx(ctx)
		if hash := chi.URLParam(r, "hash"); hash != "" {
			m.Hash = HashFromString(hash)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, ModelCtxtKey, m)))
	})
}

func MessageUserContentModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		m := ContextContentModel(ctx)
		if m == nil {
			m = new(contentModel)
			m.Content = new(Account)
		}
		authors := ContextAuthors(ctx)
		if len(authors) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		auth := authors[0]
		m.Title = fmt.Sprintf("Send user %s private message", auth.Handle)
		m.Message.Editable = true
		m.Message.Label = fmt.Sprintf("Message %s:", auth.Handle)
		m.Message.Back = PermaLink(&auth)
		m.Message.SubmitLabel = htmlf("%s Send", icon("lock"))
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, ModelCtxtKey, m)))
	})
}

func ThreadedListingMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer next.ServeHTTP(w, r)

		c := ContextCursor(r.Context())
		if c == nil {
			return
		}

		comments := make(ItemPtrCollection, 0)
		accounts := make(AccountPtrCollection, 0)
		for _, ren := range c.items {
			if it, ok := ren.(*Item); ok {
				comments = append(comments, it)
			}
			if ac, ok := ren.(*Account); ok {
				accounts = append(accounts, ac)
			}
		}

		reparentComments(&comments)
		addLevelComments(comments)

		reparentAccounts(&accounts)
		addLevelAccounts(accounts)

		newitems := make(RenderableList, 0)
		for _, ren := range c.items {
			switch ren.Type() {
			case CommentType:
				for _, it := range comments {
					if it == ren {
						newitems.Append(it)
					}
				}
			case ActorType:
				for _, ac := range accounts {
					if ac == ren {
						newitems.Append(ac)
					}
				}
			default:
				newitems.Append(ren)
			}
		}
		if len(newitems) > 0 {
			c.items = newitems
		}
	})
}

var maintenanceModel = &errorModel{
	Status: http.StatusOK,
	Title:  "Maintenance",
	Errors: []error{errors.Newf("Server in maintenance mode, please come back later.")},
}

func OutOfOrderMw(v *view) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !v.c.MaintenanceMode {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v.RenderTemplate(r, w, "error", maintenanceModel)
		})
	}
}

func SortByScore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer next.ServeHTTP(w, r)

		m := ContextListingModel(r.Context())
		if m == nil {
			return
		}
		m.sortFn = ByScore
	})
}

func SortByDate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer next.ServeHTTP(w, r)
		m := ContextListingModel(r.Context())
		if m == nil {
			return
		}
		m.sortFn = ByDate
	})
}

type deps struct {
	Votes   bool
	Authors bool
	Replies bool
	Follows bool
}

func LoadVotes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		deps := ContextDependentLoads(ctx)
		deps.Votes = true

		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, DependenciesCtxtKey, deps)))
	})
}

func LoadAuthors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		deps := ContextDependentLoads(ctx)
		deps.Authors = true

		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, DependenciesCtxtKey, deps)))
	})
}

func LoadReplies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		deps := ContextDependentLoads(ctx)
		deps.Replies = true

		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, DependenciesCtxtKey, deps)))
	})
}
