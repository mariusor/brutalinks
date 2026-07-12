package brutalinks

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"git.sr.ht/~mariusor/box"
	"git.sr.ht/~mariusor/lw"
	"git.sr.ht/~mariusor/ssm"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/filters"
	"github.com/go-ap/filters/index"
)

func logErr(err error) lw.Ctx {
	return lw.Ctx{"err": err.Error()}
}

func logIRI(iri vocab.IRI) lw.Ctx {
	return lw.Ctx{"iri": iri.String()}
}

var maxCount = 20

func firstPageIRI(toLoad vocab.IRI, before vocab.IRI) vocab.IRI {
	ff := make(filters.Checks, 0, 2)
	ff = append(ff, filters.WithMaxCount(maxCount))
	if len(before) > 0 {
		ff = append(ff, filters.Before(filters.SameID(before)))
	}

	q := filters.ToValues(ff...)
	u, _ := toLoad.URL()
	u.RawQuery = q.Encode()

	return vocab.IRI(u.String())
}

var collectionsForLoad = vocab.CollectionPaths{
	vocab.Inbox,
	vocab.Outbox,
	vocab.Followers,
	vocab.Following,
	vocab.Liked,
	vocab.Likes,
	vocab.Shares,
	// NOTE(marius): these are not taken into account yet, as they
	// need to be added to the custom logic in CollectionPath.ofActor
	// which should look at the Actor.Endpoints list.
	// They also are in use only by the main Service actor of FedBOX.
	filters.ObjectsType,
	filters.ActorsType,
	filters.ActivitiesType,
}

func getNextCollectionPage(col vocab.CollectionInterface) vocab.IRI {
	var next vocab.IRI
	if col == nil {
		return next
	}
	typ := col.GetType()
	switch {
	case vocab.OrderedCollectionPageType.Match(typ):
		if c, ok := col.(*vocab.OrderedCollectionPage); ok {
			if c.Next != nil {
				next = c.Next.GetLink()
			}
		}
	case vocab.OrderedCollectionType.Match(typ):
		if c, ok := col.(*vocab.OrderedCollection); ok {
			if c.First != nil {
				next = c.First.GetLink()
			}
		}
	case vocab.CollectionPageType.Match(typ):
		if c, ok := col.(*vocab.CollectionPage); ok {
			if c.Next != nil {
				next = c.Next.GetLink()
			}
		}
	case vocab.CollectionType.Match(typ):
		if c, ok := col.(*vocab.Collection); ok {
			if c.First != nil {
				next = c.First.GetLink()
			}
		}
	}
	return next
}

func (r *repository) Follow(ctx context.Context) error {
	return r.runDelayed(ctx)
}

var defaultDelay = 30 * time.Second

type Fetch struct {
	box.Client
	cred        *box.C2S
	C           *client.C
	Collections []vocab.IRI
	infoFn      CtxLogFn
	errFn       CtxLogFn
}

func (f *Fetch) fetchActors(ctx context.Context) ssm.Fn {
	actor, err := f.C.Actor(ctx, f.cred.IRI)
	if err != nil {
		return ssm.End
	}

	for _, col := range collectionsForLoad {
		if iri := col.Of(actor); !vocab.IsNil(iri) {
			f.Collections = append(f.Collections, iri.GetLink())
		}
	}
	return ssm.End
}

var pauseFetch atomic.Bool

func (r *repository) Paused() bool {
	return r.conf.MaintenanceMode
}

func (r *repository) loadCredentialsAndRun(ctx context.Context) ssm.Fn {
	found, _ := box.LoadAllCredentials(r.b)
	if len(found) == 0 {
		return ssm.End
	}
	if r.Paused() {
		r.infoFn(nil)("Fetch is paused, exiting")
		return ssm.End
	}

	fetches := make([]Fetch, 0, len(found))
	for _, cred := range found {
		f := Fetch{
			Client:      *r.b,
			cred:        cred,
			C:           r.b.Client(ctx, cred),
			Collections: make([]vocab.IRI, 0),
			infoFn:      r.infoFn,
			errFn:       r.errFn,
		}
		fetches = append(fetches, f)
	}

	fetchFns := make([]ssm.Fn, 0, len(fetches))
	for _, f := range fetches {
		actor, err := f.C.Actor(ctx, f.cred.IRI)
		if err != nil {
			r.errFn(logIRI(f.cred.IRI))("unable to load actor")
			continue
		}

		for _, col := range collectionsForLoad {
			if iri := col.Of(actor); !vocab.IsNil(iri) {
				f.Collections = append(f.Collections, iri.GetLink())
			}
		}

		for _, toLoad := range f.Collections {
			fetchFns = append(fetchFns, f.fetchCollection(r.st, toLoad))
		}
	}

	if len(fetchFns) == 0 {
		return ssm.End
	}
	return ssm.Batch(fetchFns...)
}

func (r *repository) runDelayed(ctx context.Context) error {
	failed := make(chan error)
	var cancelFn func()

	ctx, cancelFn = context.WithCancel(ctx)
	defer cancelFn()

	for {
		go func() {
			failed <- ssm.Run(ctx, r.loadCredentialsAndRun)
		}()
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				r.infoFn(logErr(err))("failed")
				return err
			}
			return nil
		case err := <-failed:
			if err != nil {
				r.infoFn(logErr(err))("failed")
				return err
			}
			r.infoFn(lw.Ctx{"after": defaultDelay})("Restarting")
			time.Sleep(defaultDelay)
		}
	}
}

func collectionPartOf(iri vocab.IRI) vocab.IRI {
	u, _ := iri.URL()
	u.RawQuery = ""
	return vocab.IRI(u.String())
}

func isFirstPage(iri vocab.IRI) bool {
	u, err := iri.URL()
	if err != nil {
		return false
	}
	return len(filters.PaginatorValues(u.Query())) <= 1
}

func urlMaxItems(iri vocab.IRI) int {
	u, err := iri.URL()
	if err != nil {
		return -1
	}
	return filters.PaginatorValues(u.Query()).Count()
}

func removeFromList(col []vocab.IRI, iri vocab.IRI) []vocab.IRI {
	result := make([]vocab.IRI, 0, len(col))
	for i := len(col) - 1; i >= 0; i-- {
		if ir := col[i]; ir.Equals(iri, true) {
			result = append(result, col[:i]...)
			result = append(result, col[i+1:]...)
		}
	}
	return result
}

func (f *Fetch) fetchCollection(st *Storage, toLoad vocab.IRI) ssm.Fn {
	if f.C == nil {
		f.errFn(nil)("nil ActivityPub client")
		return ssm.End
	}
	if st == nil {
		f.errFn(nil)("nil local storage")
		return ssm.End
	}

	l := f.infoFn(logIRI(toLoad), lw.Ctx{"as": f.cred.IRI})

	return func(ctx context.Context) ssm.Fn {
		//if f.Paused() {
		//	l("Fetch is paused, exiting")
		//	return ssm.End
		//}
		start := time.Now()

		l("Loading")

		colIRI := collectionPartOf(toLoad)

		m, _ := st.loadMetaData(colIRI)
		if m == nil {
			m = new(metadata)
			m.Ref = index.HashFn(colIRI)
			m.IRI = colIRI
		}
		m.UpdateTime = start
		firstPage := isFirstPage(toLoad)
		if firstPage {
			toLoad = firstPageIRI(toLoad, "")
		}

		col, err := f.C.Collection(ctx, toLoad)
		l = f.infoFn(lw.Ctx{"dur": time.Since(start)})
		if err != nil {
			f.errFn(logErr(err))("unable to load collection, removing from list")
			if firstPage {
				f.Collections = removeFromList(f.Collections, colIRI)
			}
			return ssm.End
		}

		foundTopOfCollection := false
		pageMaxItems := urlMaxItems(col.GetLink())

		toSave := make(vocab.ItemCollection, 0)
		next := make([]ssm.Fn, 0, 3)
		err = vocab.OnOrderedCollectionPage(col, func(c *vocab.OrderedCollectionPage) error {
			if !vocab.IsNil(c.PartOf) {
				colIRI = c.PartOf.GetLink()
			}

			m.Mod = c.Published
			if !c.Updated.IsZero() {
				m.Mod = c.Updated
			}

			var top vocab.IRI
			var bottom vocab.IRI
			var exhausted bool
			if firstPage {
				first := c.OrderedItems.First()
				top = m.Top
				if !vocab.IsNil(first) {
					m.Top = first.GetLink()
				}
			}

			for _, it := range c.OrderedItems {
				if top.IsValid() && top.Equals(it.GetLink(), true) {
					foundTopOfCollection = true
					break
				}

				bottom = it.GetLink()
				toSave = append(toSave, it)
			}
			if len(c.OrderedItems) < pageMaxItems {
				exhausted = true
			}

			if len(toSave) > 0 {
				m.Bottom = bottom
				m.Exhausted = exhausted
				next = append(next, saveItems(st, f.errFn, toSave...), saveMetadata(st, f.errFn, c, m))
			}

			return nil
		})
		if err != nil {
			f.errFn(logErr(err))("invalid collection loaded")
			return ssm.Batch(next...)
		}

		if foundTopOfCollection {
			l("Reached the end of the collection")
		} else if nextPage := getNextCollectionPage(col); isValidNextPage(toLoad, nextPage) {
			next = append(next, f.fetchCollection(st, nextPage))
		}

		return ssm.Batch(next...)
	}
}

func isValidNextPage(toLoad, nextPage vocab.IRI) bool {
	return nextPage.IsValid() && !nextPage.Equals(vocab.NilIRI, true) && !nextPage.Equals(toLoad, true)
}

func saveMetadata(st *Storage, l CtxLogFn, c vocab.Item, m *metadata) ssm.Fn {
	colIRI := collectionPartOf(c.GetLink())

	return func(ctx context.Context) ssm.Fn {
		if pauseFetch.Load() {
			return ssm.End
		}
		// NOTE(marius): for collections we add the items to the index in a separate step from Storage.Add()
		_ = st.in[ByCollection].Add(c)

		if m != nil {
			if err := st.saveMetaData(colIRI, *m); err != nil {
				l(logErr(err), logIRI(c.GetLink()))("failed saving metadata for item")
			}
		}
		return ssm.End
	}
}

func (r *repository) Save(items ...vocab.Item) error {
	errs := make([]error, 0)
	for _, it := range items {
		if err := r.st.Save(it); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func saveItems(st *Storage, l CtxLogFn, items ...vocab.Item) ssm.Fn {
	if len(items) == 0 {
		return ssm.End
	}

	return func(ctx context.Context) ssm.Fn {
		if pauseFetch.Load() {
			return ssm.End
		}
		_ = st.loadIndexes()
		defer st.saveIndexes()

		for _, it := range items {
			if err := st.Save(it); err != nil {
				l(logErr(err), logIRI(it.GetLink()))("failed saving item")
			}
		}

		return ssm.End
	}
}
