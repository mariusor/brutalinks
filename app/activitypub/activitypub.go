package activitypub

import (
	"fmt"

	"github.com/buger/jsonparser"
	as "github.com/mariusor/activitypub.go/activitystreams"
)

type PublicKey struct {
	ID           as.ObjectID     `jsonld:"id,omitempty"`
	Owner        as.ObjectOrLink `jsonld:"owner,omitempty"`
	PublicKeyPem string          `jsonld:"publicKeyPem,omitempty"`
}

// Person it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/actors.go#Actor
// We need it here in order to be able to add to it our Score property
type Person struct {
	as.Person
	PublicKey PublicKey `jsonld:"publicKey,omitempty"`
	// Score is our own custom property for which we needed to extend the existing AP one
	Score int64 `jsonld:"score"`
}

// Article it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/objects.go#Object
// We need it here in order to be able to add to it our Score property
type Article struct {
	as.Object
	Score int64 `jsonld:"score"`
}

// OrderedCollection it should be identical to:
//    github.com/mariusor/activitypub.go/activitystreams/collections.go#OrderedCollection
// We need it here in order to be able to implement our own UnmarshalJSON() method
type OrderedCollection as.OrderedCollection

// Activity it should be identical to:
//    github.com/mariusor/activitypub.go/activitystreams/activity.go#Activity
// We need it here in order to be able to implement our own UnmarshalJSON() method
type Activity as.Activity

func (p Person) GetID() *as.ObjectID {
	id := as.ObjectID(p.ID)
	return &id
}
func (p Person) GetType() as.ActivityVocabularyType {
	return as.ActivityVocabularyType(p.Type)
}
func (p Person) GetLink() as.IRI {
	return as.IRI(p.ID)
}
func (p Person) IsLink() bool {
	return false
}

func (p Person) IsObject() bool {
	return true
}

func (a Article) GetID() *as.ObjectID {
	id := as.ObjectID(a.ID)
	return &id
}

func (a Article) GetLink() as.IRI {
	return as.IRI(a.ID)
}
func (a Article) GetType() as.ActivityVocabularyType {
	return as.ActivityVocabularyType(a.Type)
}

func (a Article) IsLink() bool {
	return false
}

func (a Article) IsObject() bool {
	return true
}

// UnmarshalJSON
func (a *Article) UnmarshalJSON(data []byte) error {
	it := as.Object{}
	err := it.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	a.Object = it
	if score, err := jsonparser.GetInt(data, "score"); err == nil {
		a.Score = score
	}

	return nil
}

// UnmarshalJSON
func (p *Person) UnmarshalJSON(data []byte) error {
	app := as.Person{}
	err := app.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	p.Person = app
	if score, err := jsonparser.GetInt(data, "score"); err == nil {
		p.Score = score
	}

	return nil
}

// CollectionNew initializes a new Collection
func OrderedCollectionNew(id as.ObjectID) *OrderedCollection {
	o := OrderedCollection(*as.OrderedCollectionNew(id))
	return &o
}

// GetType returns the OrderedCollection's type
func (o OrderedCollection) GetType() as.ActivityVocabularyType {
	return o.Type
}

// GetLink returns the IRI of the OrderedCollection object
func (o OrderedCollection) GetLink() as.IRI {
	return as.IRI(o.ID)
}

// IsLink returns false for an OrderedCollection object
func (o OrderedCollection) IsLink() bool {
	return false
}

// GetID returns the ObjectID corresponding to the OrderedCollection
func (o OrderedCollection) GetID() *as.ObjectID {
	return &o.ID
}

// IsObject returns true for am OrderedCollection object
func (o OrderedCollection) IsObject() bool {
	return true
}

// Collection returns the underlying Collection type
func (o *OrderedCollection) Collection() as.CollectionInterface {
	return o
}

// Append adds an element to an OrderedCollection
func (o *OrderedCollection) Append(ob as.Item) error {
	o.OrderedItems = append(o.OrderedItems, ob)
	o.TotalItems++
	return nil
}

// UnmarshalJSON
func (o *OrderedCollection) UnmarshalJSON(data []byte) error {
	col := as.OrderedCollection{}
	err := col.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	var items = make(as.ItemCollection, 0)
	for i, it := range col.OrderedItems {
		var a as.ObjectOrLink
		if as.ValidActivityType(it.GetType()) {
			act := &Activity{}
			if data, _, _, err := jsonparser.Get(data, "orderedItems", fmt.Sprintf("[%d]", i)); err == nil {
				act.UnmarshalJSON(data)
			}
			if context, err := jsonparser.GetString(data, "orderedItems", fmt.Sprintf("[%d]", i), "context"); err == nil {
				act.Context = as.IRI(context)
			}
			a = act
		} else if as.ValidObjectType(it.GetType()) {
			art := &Article{}
			if data, _, _, err := jsonparser.Get(data, "orderedItems", fmt.Sprintf("[%d]", i)); err == nil {
				art.UnmarshalJSON(data)
			}
			if context, err := jsonparser.GetString(data, "orderedItems", fmt.Sprintf("[%d]", i), "context"); err == nil {
				art.Context = as.IRI(context)
			}
			a = art
		}
		if a == nil {
			continue
		}
		items = append(items, a)
	}

	*o = OrderedCollection(col)
	o.OrderedItems = items
	o.TotalItems = uint(len(items))
	return nil
}

func (a Activity) GetID() *as.ObjectID {
	id := as.ObjectID(a.ID)
	return &id
}

func (a Activity) GetLink() as.IRI {
	return as.IRI(a.ID)
}
func (a Activity) GetType() as.ActivityVocabularyType {
	return as.ActivityVocabularyType(a.Type)
}

func (a Activity) IsLink() bool {
	return false
}

func (a Activity) IsObject() bool {
	return true
}

// UnmarshalJSON
func (a *Activity) UnmarshalJSON(data []byte) error {
	it := as.Activity{}
	err := it.UnmarshalJSON(data)
	if err != nil {
		return err
	}
	*a = Activity(it)
	if _, err := jsonparser.GetString(data, "object", "type"); err == nil {
		// when we have a type try to
		// convert objects to local articles
		obj := Article{}
		if data, _, _, err := jsonparser.Get(data, "object"); err == nil {
			obj.UnmarshalJSON(data)
			a.Object = obj
		}
	}

	return nil
}
