package app

import (
	"context"
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
			authors = AccountCollection { self }
		} else {
			var err error
			repo := ContextRepository(r.Context())
			ctx, _ := context.WithTimeout(context.TODO(), time.Second)
			authors, err = repo.accounts(ctx, FilterAccountByHandle(handle))
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

func LoadOutboxMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authors := ContextAuthors(r.Context())
		if len(authors) == 0 {
			ctxtErr(next, w, r, errors.NotFoundf("actor not found"))
			return
		}
		f := ContextActivityFilters(r.Context())
		repo := ContextRepository(r.Context())

		var cursor = new(Cursor)
		cursor.items = make(RenderableList, 0)
		for _, author := range authors {
			c, err := repo.LoadActorOutbox(context.TODO(), author.pub, f...)
			if err != nil {
				repo.errFn(log.Ctx{"author": author.Handle, "err": err.Error()})("Unable to load outbox")
				continue
			}
			cursor.items.Merge(c.items)
			cursor.total += c.total
			cursor.before = c.before
			cursor.after = c.after
		}
		ctx := context.WithValue(r.Context(), CursorCtxtKey, cursor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoadInboxMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := ContextActivityFilters(r.Context())
		repo := ContextRepository(r.Context())
		acc := loggedAccount(r)
		if acc == nil {
			ctxtErr(next, w, r, errors.MethodNotAllowedf("nil account"))
			return
		}
		cursor, err := repo.LoadActorInbox(context.TODO(), acc.pub, f...)
		if err != nil {
			ctxtErr(next, w, r, errors.Annotatef(err, "unable to load current account's inbox"))
			return
		}
		ctx := context.WithValue(r.Context(), CursorCtxtKey, &cursor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoadServiceInboxMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := ContextActivityFilters(r.Context())
		repo := ContextRepository(r.Context())
		cursor, err := repo.LoadActorInbox(context.TODO(), repo.fedbox.Service(), f...)
		if err != nil {
			ctxtErr(next, w, r, errors.Annotatef(err, "unable to load the %s's inbox", repo.fedbox.Service().Type))
			return
		}
		ctx := context.WithValue(r.Context(), CursorCtxtKey, &cursor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoadServiceWithSelfAuthInboxMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := ContextActivityFilters(r.Context())
		repo := ContextRepository(r.Context())
		repo.fedbox.SignBy(repo.app)
		cursor, err := repo.LoadActorInbox(context.TODO(), repo.fedbox.Service(), f...)
		if err != nil {
			ctxtErr(next, w, r, errors.Annotatef(err, "unable to load the %s's inbox", repo.fedbox.Service().Type))
			return
		}
		ctx := context.WithValue(r.Context(), CursorCtxtKey, &cursor)
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
		logged := ContextAccount(r.Context())

		c, err := repo.ActorCollection(context.WithValue(context.TODO(), LoggedAccountCtxtKey, logged), searches)
		if err != nil {
			ctxtErr(next, w, r, errors.NotFoundf(strings.TrimLeft(r.URL.Path, "/")))
			return
		}
		ctx := context.WithValue(r.Context(), CursorCtxtKey, &c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func searchesInCollectionsMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repo := ContextRepository(r.Context())
		current := ContextAccount(r.Context())
		ff := ContextActivityFilters(r.Context())

		service := repo.fedbox.Service().GetLink()
		searchIn := RemoteLoads{
			service: []RemoteLoad{{actor: repo.fedbox.Service(), loadFn: inbox, filters: ff}},
		}
		if current.IsLogged() {
			searchIn[service] = append(
				searchIn[service],
				RemoteLoad{actor: current.pub, loadFn: inbox, filters: ff},
				RemoteLoad{actor: current.pub, loadFn: outbox, filters: ff},
			)
		}
		rtx := context.WithValue(r.Context(), LoadsCtxtKey, searchIn)
		next.ServeHTTP(w, r.WithContext(rtx))
	})
}

func LoadSingleItemRepliesMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := context.TODO()

		deps := ContextDependentLoads(r.Context())
		repo := ContextRepository(r.Context())
		item := ContextItem(r.Context())
		if err != nil || !item.IsValid() {
			var ctx log.Ctx
			if err != nil {
				ctx = log.Ctx{"err": err.Error()}
			}
			repo.errFn(ctx)("item not found")
			ctxtErr(next, w, r, errors.NotFoundf(strings.TrimLeft(r.URL.Path, "/")))
			return
		}

		items := ItemCollection{*item}
		if deps.Replies {
			if comments, err := repo.loadItemsReplies(ctx, items...); err == nil {
				items = append(items, comments...)
			}
		}
		if deps.Authors {
			if items, err = repo.loadItemsAuthors(ctx, items...); err != nil {
				repo.errFn()("unable to load item authors")
			}
		}
		if deps.Votes {
			if items, err = repo.loadItemsVotes(ctx, items...); err != nil {
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
		var err error

		ctx := context.TODO()

		repo := ContextRepository(r.Context())
		searchIn := ContextLoads(r.Context())

		var item Item
		err = LoadFromSearches(ctx, repo, searchIn, func(ctx context.Context, c pub.CollectionInterface, f *Filters) error {
			for _, it := range c.Collection() {
				err := pub.OnActivity(it, func(act *pub.Activity) error {
					if act.Object.IsLink() {
						ob, err := repo.fedbox.Object(ctx, act.Object.GetLink())
						if err != nil {
							repo.errFn(log.Ctx{"iri": act.Object.GetLink()})("unable to load item")
							return err
						}
						it = ob
					}
					if err := item.FromActivityPub(act.Object); err != nil {
						repo.errFn(log.Ctx{"iri": act.Object.GetLink()})("unable to load item")
						return err
					}
					return nil
				})
				if err != nil {
					repo.errFn(log.Ctx{"iri": it.GetLink()})("unable to load item")
					return err
				}
				if item.IsValid() {
					ctx.Done()
					break
				}
			}
			return nil
		})
		if err != nil || !item.IsValid() {
			var ctx log.Ctx
			if err != nil {
				ctx = log.Ctx{"err": err.Error()}
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

func OutOfOrderMw (v *view) func (next http.Handler) http.Handler {
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
}

func LoadItemsVotes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		deps := ContextDependentLoads(ctx)
		deps.Votes = true

		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, DependenciesCtxtKey, deps)))
	})
}

func LoadItemsAuthors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		deps := ContextDependentLoads(ctx)
		deps.Authors = true

		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, DependenciesCtxtKey, deps)))
	})
}

func LoadItemsReplies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		deps := ContextDependentLoads(ctx)
		deps.Replies = true

		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, DependenciesCtxtKey, deps)))
	})
}
