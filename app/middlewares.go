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
		f := ContextActivityFilters(r.Context())
		repo := ContextRepository(r.Context())
		cursor, err := repo.LoadActorOutbox(author.pub, f)
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

		ctx := context.WithValue(r.Context(), CursorCtxtKey, &c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
