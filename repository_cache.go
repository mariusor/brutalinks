package brutalinks

import (
	"context"
	"path"
	"time"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/cache"
	"github.com/go-ap/client"
)

func caches(enabled bool) *cc {
	c := cache.New(enabled)
	return &cc{c}
}

type cc struct {
	c cache.CanStore
}

func accum(toRemove *vocab.IRIs, iri vocab.IRI, col vocab.CollectionPath) {
	if repl := col.IRI(iri); !toRemove.Contains(repl) {
		*toRemove = append(*toRemove, repl)
	}
}

func accumItem(it vocab.Item, toRemove *vocab.IRIs, col vocab.CollectionPath) {
	if vocab.IsNil(it) {
		return
	}
	if vocab.IsItemCollection(it) {
		vocab.OnItemCollection(it, func(c *vocab.ItemCollection) error {
			for _, ob := range c.Collection() {
				accum(toRemove, ob.GetLink(), col)
			}
			return nil
		})
	} else {
		accum(toRemove, it.GetLink(), col)
	}
}

func (c *cc) removeRelated(items ...vocab.Item) {
	toRemove := make(vocab.IRIs, 0)
	for _, it := range items {
		if vocab.IsNil(it) {
			continue
		}
		if vocab.IsObject(it) || vocab.IsItemCollection(it) && len(it.GetLink()) > 0 {
			typ := it.GetType()
			if vocab.ActivityTypes.Contains(typ) || vocab.IntransitiveActivityTypes.Contains(typ) {
				vocab.OnActivity(it, c.accumActivityIRIs(&toRemove))
			} else {
				vocab.OnObject(it, c.accumObjectIRIs(&toRemove))
			}
		}

		if aIRI := it.GetLink(); len(aIRI) > 0 && !toRemove.Contains(aIRI) {
			toRemove = append(toRemove, aIRI)
		}
	}
	c.remove(toRemove...)
}

func (c *cc) accumRecipientIRIs(r vocab.Item, toRemove *vocab.IRIs) {
	iri := r.GetLink()
	if iri.Equals(vocab.PublicNS, false) {
		return
	}

	_, col := vocab.Split(iri)

	toDeref := vocab.CollectionPaths{vocab.Followers, vocab.Following}
	if toDeref.Contains(col) {
		if iris, isCached := c.get(iri); isCached {
			vocab.OnCollectionIntf(iris, func(col vocab.CollectionInterface) error {
				for _, it := range col.Collection() {
					accumItem(it.GetLink(), toRemove, vocab.Outbox)
				}
				return nil
			})
		}
		return
	}
	toAppend := vocab.CollectionPaths{vocab.Inbox, vocab.Outbox}
	if toAppend.Contains(col) {
		if toRemove.Contains(iri) {
			*toRemove = append(*toRemove, iri)
		}
		return
	}
	accumItem(r, toRemove, vocab.Inbox)
}

func (c *cc) accumActivityIRIs(toRemove *vocab.IRIs) func(activity *vocab.Activity) error {
	return func(a *vocab.Activity) error {
		for _, r := range a.Recipients() {
			c.accumRecipientIRIs(r, toRemove)
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
		return vocab.OnObject(a.Object, c.accumObjectIRIs(toRemove))
	}
}

func (c *cc) accumObjectIRIs(toRemove *vocab.IRIs) func(*vocab.Object) error {
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
		for _, r := range ob.Recipients() {
			c.accumRecipientIRIs(r, toRemove)
		}
		accumItem(ob.InReplyTo, toRemove, vocab.Replies)
		accumItem(ob.AttributedTo, toRemove, vocab.Outbox)
		return nil
	}
}

func (c *cc) remove(iris ...vocab.IRI) {
	if len(iris) == 0 {
		return
	}
	c.c.Delete(iris...)
}

func (c *cc) add(iri vocab.IRI, it vocab.Item) {
	c.c.Store(iri, it)
}

func (c *cc) get(iri vocab.IRI) (vocab.Item, bool) {
	it := c.c.Load(iri)
	return it, !vocab.IsNil(it)
}

func (c *cc) loadFromSearches(repo *repository, search RemoteLoads) error {
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
