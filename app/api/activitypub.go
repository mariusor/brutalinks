package api

import (
	"crypto"
	"fmt"
	"net/http"

	"github.com/buger/jsonparser"
	as "github.com/mariusor/activitypub.go/activitystreams"
	"github.com/spacemonkeygo/httpsig"
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
//    github.com/mariusor/activitypub.go/activitypub/collections.go#OrderedCollection
// We need it here in order to be able to implement our own UnmarshalJSON() method
type OrderedCollection as.OrderedCollection

func (p Person) GetID() *as.ObjectID {
	id := as.ObjectID(p.ID)
	return &id
}
func (p Person) GetType() as.ActivityVocabularyType {
	return as.ActivityVocabularyType(p.Type)
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

	items := make(as.ItemCollection, col.TotalItems)
	for i, it := range col.OrderedItems {
		var a as.ObjectOrLink
		switch it.GetType() {
		case as.ArticleType:
			art := &Article{}
			if data, _, _, err := jsonparser.Get(data, "orderedItems", fmt.Sprintf("[%d]", i)); err == nil {
				art.UnmarshalJSON(data)
			}
			if context, err := jsonparser.GetString(data, "orderedItems", fmt.Sprintf("[%d]", i), "context"); err == nil {
				art.Context = as.IRI(context)
			}
			a = art
		case as.LikeType:
			fallthrough
		case as.DislikeType:
			fallthrough
		case as.ActivityType:
			act := &as.Activity{}
			if data, _, _, err := jsonparser.Get(data, "orderedItems", fmt.Sprintf("[%d]", i)); err == nil {
				act.UnmarshalJSON(data)
			}
			if context, err := jsonparser.GetString(data, "orderedItems", fmt.Sprintf("[%d]", i), "context"); err == nil {
				act.Context = as.IRI(context)
			}
			a = act
		}
		items[i] = a
	}
	col.OrderedItems = items

	*o = OrderedCollection(col)
	return nil
}

type SignFunc func(r *http.Request) error

func SignRequest(r *http.Request, p Person, key crypto.PrivateKey) error {
	hdrs := []string{"(request-target)", "host", "test", "date"}

	return httpsig.NewSigner(string(p.PublicKey.ID), key, httpsig.RSASHA256, hdrs).
		Sign(r)
}
