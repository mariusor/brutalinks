package api

import (
	"crypto"
	"fmt"
	"github.com/buger/jsonparser"
	ap "github.com/mariusor/activitypub.go/activitypub"
	"github.com/spacemonkeygo/httpsig"
	"net/http"
)

type PublicKey struct {
	ID           ap.ObjectID     `jsonld:"id,omitempty"`
	Owner        ap.ObjectOrLink `jsonld:"owner,omitempty"`
	PublicKeyPem string          `jsonld:"publicKeyPem,omitempty"`
}

// Person it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/actors.go#Actor
// We need it here in order to be able to add to it our Score property
type Person struct {
	ap.Person
	PublicKey PublicKey `jsonld:"publicKey,omitempty"`
	// Score is our own custom property for which we needed to extend the existing AP one
	Score int64 `jsonld:"score"`
}

// Article it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/objects.go#Object
// We need it here in order to be able to add to it our Score property
type Article struct {
	ap.Object
	Score int64 `jsonld:"score"`
}

// OrderedCollection it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/collections.go#OrderedCollection
// We need it here in order to be able to implement our own UnmarshalJSON() method
type OrderedCollection ap.OrderedCollection

func (p Person) GetID() *ap.ObjectID {
	id := ap.ObjectID(p.ID)
	return &id
}
func (p Person) GetType() ap.ActivityVocabularyType {
	return ap.ActivityVocabularyType(p.Type)
}
func (p Person) IsLink() bool {
	return false
}
func (p Person) IsObject() bool {
	return true
}

func (a Article) GetID() *ap.ObjectID {
	id := ap.ObjectID(a.ID)
	return &id
}
func (a Article) GetType() ap.ActivityVocabularyType {
	return ap.ActivityVocabularyType(a.Type)
}
func (a Article) IsLink() bool {
	return false
}

func (a Article) IsObject() bool {
	return true
}

// UnmarshalJSON
func (a *Article) UnmarshalJSON(data []byte) error {
	it := ap.Object{}
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
	app := ap.Person{}
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
func OrderedCollectionNew(id ap.ObjectID) *OrderedCollection {
	o := OrderedCollection(*ap.OrderedCollectionNew(id))
	return &o
}

// GetType returns the OrderedCollection's type
func (o OrderedCollection) GetType() ap.ActivityVocabularyType {
	return o.Type
}

// IsLink returns false for an OrderedCollection object
func (o OrderedCollection) IsLink() bool {
	return false
}

// GetID returns the ObjectID corresponding to the OrderedCollection
func (o OrderedCollection) GetID() *ap.ObjectID {
	return &o.ID
}

// IsObject returns true for am OrderedCollection object
func (o OrderedCollection) IsObject() bool {
	return true
}
// Collection returns the underlying Collection type
func (o *OrderedCollection) Collection() ap.CollectionInterface {
	return o
}

// Append adds an element to an OrderedCollection
func (o *OrderedCollection) Append(ob ap.Item) error {
	o.OrderedItems = append(o.OrderedItems, ob)
	o.TotalItems++
	return nil
}

// UnmarshalJSON
func (o *OrderedCollection) UnmarshalJSON(data []byte) error {
	col := ap.OrderedCollection{}
	err := col.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	items := make(ap.ItemCollection, col.TotalItems)
	for i, it := range col.OrderedItems {
		var a ap.ObjectOrLink
		switch it.GetType() {
		case ap.ArticleType:
			art := &Article{}
			if data, _, _, err := jsonparser.Get(data, "orderedItems", fmt.Sprintf("[%d]", i)); err == nil {
				art.UnmarshalJSON(data)
			}
			if context, err := jsonparser.GetString(data, "orderedItems", fmt.Sprintf("[%d]", i), "context"); err == nil {
				art.Context = ap.IRI(context)
			}
			a = art
		case ap.LikeType:
			fallthrough
		case ap.DislikeType:
			fallthrough
		case ap.ActivityType:
			act := &ap.Activity{}
			if data, _, _, err := jsonparser.Get(data, "orderedItems", fmt.Sprintf("[%d]", i)); err == nil {
				act.UnmarshalJSON(data)
			}
			if context, err := jsonparser.GetString(data, "orderedItems", fmt.Sprintf("[%d]", i), "context"); err == nil {
				act.Context = ap.IRI(context)
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
