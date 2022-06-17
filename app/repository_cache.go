package app

import (
	"context"
	"path"
	"sync"
	"time"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
)

type cacheEntries map[vocab.IRI]vocab.Item

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

func removeAccum(toRemove *vocab.IRIs, iri vocab.IRI, col vocab.CollectionPath) {
	if repl := col.IRI(iri); !toRemove.Contains(repl) {
		*toRemove = append(*toRemove, repl)
	}
}

func accumForProperty(it vocab.Item, toRemove *vocab.IRIs, col vocab.CollectionPath) {
	if vocab.IsNil(it) {
		return
	}
	if vocab.IsItemCollection(it) {
		vocab.OnItemCollection(it, func(c *vocab.ItemCollection) error {
			for _, ob := range c.Collection() {
				removeAccum(toRemove, ob.GetLink(), col)
			}
			return nil
		})
	} else {
		removeAccum(toRemove, it.GetLink(), col)
	}
}

func (c *cache) removeRelated(items ...vocab.Item) {
	toRemove := make(vocab.IRIs, 0)
	for _, it := range items {
		if vocab.IsNil(it) {
			continue
		}
		if vocab.IsObject(it) || vocab.IsItemCollection(it) {
			typ := it.GetType()
			if vocab.ActivityTypes.Contains(typ) || vocab.IntransitiveActivityTypes.Contains(typ) {
				vocab.OnActivity(it, aggregateActivityIRIs(&toRemove))
			} else {
				vocab.OnObject(it, aggregateObjectIRIs(&toRemove))
			}
		}

		if aIRI := it.GetLink(); len(aIRI) > 0 && !toRemove.Contains(aIRI) {
			toRemove = append(toRemove, aIRI)
		}
	}
	c.remove(toRemove...)
}

func aggregateActivityIRIs(toRemove *vocab.IRIs) func(activity *vocab.Activity) error {
	return func(a *vocab.Activity) error {
		for _, r := range a.Recipients() {
			if r.GetLink().Equals(vocab.PublicNS, false) {
				continue
			}
			if iri := r.GetLink(); vocab.ValidCollectionIRI(iri) {
				// TODO(marius): for followers, following collections this should dereference the members
				if !toRemove.Contains(iri) {
					*toRemove = append(*toRemove, iri)
				}
			} else {
				accumForProperty(r, toRemove, vocab.Inbox)
			}
		}
		if destCol := vocab.Outbox.IRI(a.Actor); !toRemove.Contains(destCol) {
			*toRemove = append(*toRemove, destCol)
		}
		typ := a.Type
		withSideEffects := vocab.ActivityVocabularyTypes{vocab.UpdateType, vocab.UndoType, vocab.DeleteType}
		if withSideEffects.Contains(typ) {
			base := path.Dir(a.Object.GetLink().String())
			*toRemove = append(*toRemove, vocab.IRI(base), a.Object.GetLink())
		}
		return vocab.OnObject(a.Object, aggregateObjectIRIs(toRemove))
	}
}

func aggregateObjectIRIs(toRemove *vocab.IRIs) func(*vocab.Object) error {
	return func(ob *vocab.Object) error {
		if ob == nil {
			return nil
		}
		if !ob.IsObject() {
			return nil
		}
		if obIRI := ob.GetLink(); len(obIRI) > 0 && !toRemove.Contains(obIRI) {
			*toRemove = append(*toRemove, obIRI)
		}
		accumForProperty(ob.InReplyTo, toRemove, vocab.Replies)
		accumForProperty(ob.AttributedTo, toRemove, vocab.Outbox)
		return nil
	}
}

func (c *cache) remove(iris ...vocab.IRI) {
	removeForIRI := func(toRemove vocab.IRI) bool {
		if len(iris) == 0 {
			return true
		}
		for _, iri := range iris {
			if iri.Equals(toRemove, false) {
				return true
			}
		}
		return false
	}
	for k := range c.m {
		if removeForIRI(k) {
			delete(c.m, k)
		}
	}
}

func (c *cache) add(iri vocab.IRI, it vocab.Item) {
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

func (c *cache) get(iri vocab.IRI) (vocab.Item, bool) {
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
	ctx, _ := context.WithTimeout(context.TODO(), time.Second)
	return LoadFromSearches(ctx, repo, search, func(_ context.Context, col vocab.CollectionInterface, f *Filters) error {
		c.add(col.GetLink(), col)
		for _, it := range col.Collection() {
			c.add(it.GetLink(), it)
		}
		return nil
	})
}

func colIRI(hc vocab.CollectionPath) func(it vocab.Item, fn ...client.FilterFn) vocab.IRI {
	return func(it vocab.Item, fn ...client.FilterFn) vocab.IRI {
		return iri(hc.IRI(it), fn...)
	}
}

func WarmupCaches(r *repository, self vocab.Item) error {
	f := new(Filters)
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
