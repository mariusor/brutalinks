package app

import (
	"context"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"net/http"
	"path"
)

func (h handler) LoadAuthorMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		author, err := accountFromRequestHandle(r)
		if err != nil {
			h.ErrorHandler(err).ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), AuthorCtxtKey, author)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoadOutboxMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		author := ContextAuthor(r.Context())
		if author == nil {
			next.ServeHTTP(w, r)
			return
		}
		f := ContextActivityFilters(r.Context())
		repo := ContextRepository(r.Context())
		cursor, err := repo.LoadActorOutbox(author.pub, f)
		if err != nil {
			ctxtErr(next, w, r, errors.Annotatef(err, "unable to load actor's outbox"))
			return
		}
		ctx := context.WithValue(r.Context(), CursorCtxtKey, cursor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoadInboxMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acc := loggedAccount(r)
		f := ContextActivityFilters(r.Context())
		repo := ContextRepository(r.Context())
		cursor, err := repo.LoadActorInbox(acc.pub, f)
		if err != nil {
			// @todo(marius): err
			next.ServeHTTP(w, r)
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
		cursor, err := repo.LoadActorInbox(repo.fedbox.Service(), f)
		if err != nil {
			ctxtErr(next, w, r, errors.Annotatef(err, "unable to load actor's inbox"))
			return
		}
		ctx := context.WithValue(r.Context(), CursorCtxtKey, cursor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ctxtErr(next http.Handler, w http.ResponseWriter, r *http.Request, err error) {
	status, _ := errors.HttpErrors(err)
	ctx := context.WithValue(r.Context(), ModelCtxtKey, errorModel{
		Status:     status,
		Title:      fmt.Sprintf("Error %d", status),
		StatusText: http.StatusText(status),
		Errors:     []error{err},
	})
	next.ServeHTTP(w, r.WithContext(ctx))
}

func LoadOutboxObjectMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := ContextActivityFilters(r.Context())
		repo := ContextRepository(r.Context())
		author := ContextAuthor(r.Context())
		if author == nil {
			ctxtErr(next, w, r, errors.NotFoundf("Author not found"))
			return
		}
		// @todo(marius): we should improve activities objects, as if we edit an object,
		//   we need to update the activity to contain the new object.
		col, err := repo.fedbox.Outbox(author.pub, Values(f))
		if err != nil || col == nil {
			ctxtErr(next, w, r, errors.NotFoundf("Object not found"))
			return
		}
		i := Item{}
		pub.OnOrderedCollection(col, func(c *pub.OrderedCollection) error {
			i.FromActivityPub(c.OrderedItems.First())
			return nil
		})
		if !i.IsValid() {
			repo.errFn("unable to load item", nil)
			ctxtErr(next, w, r, errors.NotFoundf("Object not found"))
			return
		}

		// @todo(marius): this is a very ugly way of checking for edit,
		//   as if we add a middleware which sets the editable bit to true,
		//   it's being run _before_ actually loading the object
		if path.Base(r.URL.Path) == "edit" {
			i.Edit = true
		}
		items := ItemCollection{i}
		if comments, err := repo.loadItemsReplies(i); err == nil {
			items = append(items, comments...)
		}
		if items, err = repo.loadItemsAuthors(items...); err != nil {
			repo.errFn("unable to load item authors", nil)
		}
		if items, err = repo.loadItemsVotes(items...); err != nil {
			repo.errFn("unable to load item votes", nil)
		}
		c := Cursor{
			items: make(RenderableList, len(items)),
		}
		for k := range items {
			c.items[k] = Renderable(&items[k])
		}
		m := ContextContentModel(r.Context())
		m.Title = fmt.Sprintf("Replies to %s item", genitive(author.Handle))
		if len(i.Title) > 0 {
			m.Title = fmt.Sprintf("%s: %s", m.Title, i.Title)
		}
		ctx := context.WithValue(context.WithValue(r.Context(), CursorCtxtKey, &c), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ThreadedListingMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := ContextCursor(r.Context())
		if c == nil {
			ctxtErr(next, w, r, errors.NotFoundf("Cursor not found"))
			next.ServeHTTP(w, r)
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

		newitems := make([]Renderable, 0)
		for _, ren := range c.items {
			switch ren.Type() {
			case Comment:
				for _, it := range comments {
					if it == ren {
						newitems = append(newitems, it)
					}
				}
			case Actor:
				for _, ac := range accounts {
					if ac == ren {
						newitems = append(newitems, ac)
					}
				}
			default:
				newitems = append(newitems, ren)
			}
		}
		if len(newitems) > 0 {
			c.items = newitems
		}
		next.ServeHTTP(w, r)
	})
}
