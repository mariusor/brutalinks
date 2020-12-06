package app

import (
	"context"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"html/template"
	"net/http"
)

func (h handler) LoadAuthorMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authors, err := accountsFromRequestHandle(r)
		if err != nil {
			h.ErrorHandler(err).ServeHTTP(w, r)
			return
		}
		storAuthors := make([]*Account, 0)
		if len(authors) == 0 {
			storAuthors = append(storAuthors, &AnonymousAccount)
		} else {
			for _, auth := range authors {
				storAuthors = append(storAuthors, auth)
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), AuthorCtxtKey, storAuthors)))
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
			if c, err := repo.LoadAccountWithDetails(context.TODO(), author, f...); err == nil {
				cursor.items.Merge(c.items)
				cursor.total += c.total
				cursor.before = c.before
				cursor.after = c.after
			}
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
			typ := "current account"
			if acc.pub != nil {
				typ = string(acc.pub.GetType())
			}
			ctxtErr(next, w, r, errors.Annotatef(err, "unable to load %s inbox", genitive(typ)))
			return
		}
		ctx := context.WithValue(r.Context(), CursorCtxtKey, cursor)
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
		ctx := context.WithValue(r.Context(), CursorCtxtKey, cursor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ctxtErr(next http.Handler, w http.ResponseWriter, r *http.Request, err error) {
	status, _ := errors.HttpErrors(err)
	ctx := context.WithValue(r.Context(), ModelCtxtKey, &errorModel{
		Status:     status,
		Title:      fmt.Sprintf("Error %d", status),
		StatusText: http.StatusText(status),
		Errors:     []error{err},
	})
	next.ServeHTTP(w, r.WithContext(ctx))
}

func LoadObjectFromInboxMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		var col pub.CollectionInterface

		ff := ContextActivityFilters(r.Context())
		repo := ContextRepository(r.Context())
		ctx := context.TODO()

		if len(ff) == 0 {
			ctxtErr(next, w, r, errors.Newf("invalid filter"))
			return
		}
		f := ff[0]
		// we first try to load from the service's inbox
		col, err = repo.fedbox.Inbox(ctx, repo.fedbox.Service(), Values(f))
		if err != nil {
			// log
			return
		}
		if col.Count() == 0 {
			// if nothing found, try to load from the logged account's collections
			current := ContextAccount(r.Context())
			if current.IsLogged() {
				// if the current user is logged, try to load from their inbox
				col, err = repo.fedbox.Inbox(ctx, current.pub, Values(f))
				if err != nil {
					// log
				}
				if col == nil || col.Count() == 0 {
					// if the current user is logged, try to load from their outbox
					col, err = repo.fedbox.Outbox(ctx, current.pub, Values(f))
					if err != nil {
						// log
						return
					}
				}
			}
		}
		i := Item{}
		if col != nil {
			pub.OnOrderedCollection(col, func(c *pub.OrderedCollection) error {
				i.FromActivityPub(c.OrderedItems.First())
				return nil
			})
		}
		if !i.IsValid() {
			repo.errFn()("unable to load item")
			ctxtErr(next, w, r, errors.NotFoundf("Object not found"))
			return
		}
		items := ItemCollection{i}
		if comments, err := repo.loadItemsReplies(ctx, i); err == nil {
			items = append(items, comments...)
		}
		if items, err = repo.loadItemsAuthors(ctx, items...); err != nil {
			repo.errFn()("unable to load item authors")
		}
		if items, err = repo.loadItemsVotes(ctx, items...); err != nil {
			repo.errFn()("unable to load item votes")
		}
		c := &Cursor{
			items: make(RenderableList),
		}
		for k := range items {
			c.items.Append(Renderable(&items[k]))
		}
		m := ContextContentModel(r.Context())
		m.Title = fmt.Sprintf("Replies to %s item", genitive(i.SubmittedBy.Handle))
		if len(i.Title) > 0 {
			m.Title = fmt.Sprintf("%s: %s", m.Title, i.Title)
		}
		rtx := context.WithValue(r.Context(), CursorCtxtKey, c)
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
			m.User = auth
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
		hash := chi.URLParam(r, "hash")
		if hash != "" {
			m.Hash = HashFromString(hash)
		}

		m.Title = "Report item"
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, ModelCtxtKey, m)))
	})
}

func ReportAccountModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		m := reportModelFromCtx(ctx)
		authors := ContextAuthors(ctx)
		if len(authors) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		auth := authors[0]
		m.Content.Object = auth
		m.Hash = auth.Hash
		m.Title = fmt.Sprintf("Report %s", auth.Handle)
		m.Message.Label = fmt.Sprintf("Report %s:", auth.Handle)
		m.Title = "Report account"
		m.Message.Back = PermaLink(auth)
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, ModelCtxtKey, m)))
	})
}

func blockModelFromCtx(ctx context.Context) *moderationModel {
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
		authors := ContextAuthors(ctx)
		if len(authors) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		auth := authors[0]
		m.Content.Object = auth
		m.Title = fmt.Sprintf("Block %s", auth.Handle)
		m.Message.Label = fmt.Sprintf("Block %s:", auth.Handle)
		m.Message.Back = PermaLink(auth)
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, ModelCtxtKey, m)))
	})
}

func BlockContentModelMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		m := blockModelFromCtx(ctx)
		hash := chi.URLParam(r, "hash")
		if hash != "" {
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
		m.Message.Back = PermaLink(auth)
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

func (h *handler) OutOfOrderMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.conf.MaintenanceMode {
			next.ServeHTTP(w, r)
			return
		}
		em := errorModel{
			Status: http.StatusOK,
			Title:  "Maintenance",
			Errors: []error{errors.Newf("Server in maintenance mode, please come back later.")},
		}
		h.v.RenderTemplate(r, w, "error", &em)
	})
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
