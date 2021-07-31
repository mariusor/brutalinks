package app

import (
	"context"
	"github.com/go-ap/client"
	"github.com/go-ap/handlers"
	"sync"

	pub "github.com/go-ap/activitypub"
)

type cacheEntries map[pub.IRI]pub.Item

func caches(enabled bool) *cache {
	f := new(cache)
	f.enabled = enabled
	return f
}

type cache struct {
	enabled bool
	m       cacheEntries
	s       sync.RWMutex
}

func (c *cache) add(iri pub.IRI, it pub.Item) {
	if !c.enabled {
		return
	}
	if c.m == nil {
		c.m = make(cacheEntries)
	}
	c.s.Lock()
	defer c.s.Unlock()

	c.m[iri] = it
}

func (c *cache) get(iri pub.IRI) (pub.Item, bool) {
	if !c.enabled {
		return nil, false
	}
	c.s.RLock()
	defer c.s.RUnlock()

	it, ok := c.m[iri]
	return it, ok
}

func (c *cache) loadFromSearches(repo *repository, search RemoteLoads) error {
	if !c.enabled {
		return nil
	}
	ctx := context.TODO()
	return LoadFromSearches(ctx, repo, search, func (col pub.CollectionInterface) error {
		for _, it := range col.Collection() {
			c.add(it.GetLink(), it)
		}
		return nil
	})
}

func colIRI (hc handlers.CollectionType) func(it pub.Item, fn ...client.FilterFn) pub.IRI {
	return func(it pub.Item, fn ...client.FilterFn) pub.IRI {
		return iri(hc.IRI(it), fn...)
	}
}
func (r *repository) WarmupCaches(self pub.Item) error {
	f := new(Filters)
	f.MaxItems = 250
	r.infoFn()("Warming up caches")

	search := RemoteLoads{
		self.GetLink(): []RemoteLoad{
			{actor: r.fedbox.Service(), loadFn: colIRI(actors), filters: []*Filters{f}},
			{actor: r.fedbox.Service(), loadFn: colIRI(objects), filters: []*Filters{f}},
			{actor: r.fedbox.Service(), loadFn: colIRI(activities), filters: []*Filters{f}},
		},
	}
	return r.cache.loadFromSearches(r, search)
}
