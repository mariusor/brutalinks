package app

import (
	"context"
	pub "github.com/go-ap/activitypub"
	"net/http"
)

func LoadAuthorMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		author, err := accountFromRequestHandle(r)
		if err != nil {
			next.ServeHTTP(w, r)
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
			// TODO(marius): error
			next.ServeHTTP(w, r)
			return
		}
		acc := loggedAccount(r)
		f := ContextActivityFilters(r.Context())
		repo := ContextRepository(r.Context())
		cursor, err := repo.LoadActorOutbox(acc.pub, f)
		if err != nil {
			// @TODO err
			next.ServeHTTP(w, r)
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
			// @TODO err
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), CursorCtxtKey, cursor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoadOutboxObjectMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := ContextActivityFilters(r.Context())
		repo := ContextRepository(r.Context())
		author := ContextAuthor(r.Context())
		if author == nil {
			// @TODO err
			next.ServeHTTP(w, r)
			return
		}
		col, err := repo.fedbox.Outbox(author.pub, Values(f))
		if err != nil || col == nil {
			// @TODO err
			next.ServeHTTP(w, r)
			return
		}
		i := Item{}
		pub.OnOrderedCollection(col, func(c *pub.OrderedCollection) error {
			i.FromActivityPub(c.OrderedItems.First())
			return nil
		})
		var items ItemCollection
		items, err = repo.loadItemsAuthors(i)
		if err != nil {
			// @TODO err
			next.ServeHTTP(w, r)
			return
		}
		items, err = repo.loadItemsVotes(items...)
		if err != nil {
			// @TODO err
			next.ServeHTTP(w, r)
			return
		}
		c := Cursor{
			items: []Renderable{items[0]},
		}

		f = FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		f.Object = &ActivityFilters{}
		f.Object.OP = CompStrs{LikeString(i.Hash.String())}
		
		ctx := context.WithValue(r.Context(), CursorCtxtKey, &c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
