package activitypub

import (
	"github.com/buger/jsonparser"
	ap "github.com/go-ap/activitypub"
	as "github.com/go-ap/activitystreams"
	"github.com/go-ap/auth"
	"github.com/go-ap/errors"
)

// Actor it should be identical to:
//    github.com/go-ap/activitypub/actors.go#actor
// We need it here in order to be able to add to it our Score property
type Actor struct {
	auth.Person
	// Score is our own custom property for which we needed to extend the existing AP one
	Score int64 `jsonld:"score"`
}

// Object it should be identical to:
//    github.com/go-ap/activitypub/objects.go#Object
// We need it here in order to be able to add to it our Score property
type Object struct {
	ap.Object
	Score int64 `jsonld:"score"`
}

// GetID returns the ObjectID pointer of current Actor instance
func (a Actor) GetID() *as.ObjectID {
	id := a.ID
	return &id
}
func (a Actor) GetType() as.ActivityVocabularyType {
	return a.Type
}
func (a Actor) GetLink() as.IRI {
	return as.IRI(a.ID)
}
func (a Actor) IsLink() bool {
	return false
}

func (a Actor) IsObject() bool {
	return true
}

// GetID returns the ObjectID pointer of current Object instance
func (a Object) GetID() *as.ObjectID {
	id := as.ObjectID(a.ID)
	return &id
}

// GetLink returns the IRI of the Object object
func (a Object) GetLink() as.IRI {
	return as.IRI(a.ID)
}

// GetType returns the current Object object's type
func (a Object) GetType() as.ActivityVocabularyType {
	return a.Type
}

// IsLink returns false for an Object object
func (a Object) IsLink() bool {
	return false
}

// IsObject returns true for an Object object
func (a Object) IsObject() bool {
	return true
}

// UnmarshalJSON tries to load json data to Object object
func (a *Object) UnmarshalJSON(data []byte) error {
	ob := ap.Object{}
	err := ob.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	a.Object = ob
	if score, err := jsonparser.GetInt(data, "score"); err == nil {
		a.Score = score
	}

	return nil
}

// UnmarshalJSON tries to load json data to Actor object
func (a *Actor) UnmarshalJSON(data []byte) error {
	p := auth.Person{}
	if err := p.UnmarshalJSON(data); err != nil {
		return err
	}

	a.Person = p
	if score, err := jsonparser.GetInt(data, "score"); err == nil {
		a.Score = score
	}

	return nil
}

// JSONGetItemByType
func JSONGetItemByType(typ as.ActivityVocabularyType) (as.Item, error) {
	if as.ActorTypes.Contains(typ) {
		act := &Actor{}
		act.Type = typ
		return act, nil
	} else if as.ObjectTypes.Contains(typ) {
		ob := &Object{}
		ob.Type = typ
		return ob, nil
	}
	return as.JSONGetItemByType(typ)
}

// ToObject
func ToObject(it as.Item) (*Object, error) {
	switch i := it.(type) {
	case *Object:
		return i, nil
	case Object:
		return &i, nil
	default:
		ob, err := as.ToObject(it)
		if err != nil {
			return nil, err
		}
		return &Object{
			Object: ap.Object{
				Parent: *ob,
			},
		}, nil
	}
	return nil, errors.Newf("unable to convert object")
}

type withObjectFn func(*Object) error

// OnObject
func OnObject(it as.Item, fn withObjectFn) error {
	ob, err := ToObject(it)
	if err != nil {
		return err
	}
	return fn(ob)
}

// ToActor
func ToActor(it as.Item) (*Actor, error) {
	switch i := it.(type) {
	case *Actor:
		return i, nil
	case Actor:
		return &i, nil
	default:
		pers, err := ap.ToPerson(it)
		if err != nil {
			return nil, err
		}
		return &Actor{
			Person: auth.Person{
				Person: *pers,
			},
		}, nil
	}
	return nil, errors.Newf("unable to convert object")
}

type withPersonFn func(*Actor) error

// OnActor
func OnActor(it as.Item, fn withPersonFn) error {
	ob, err := ToActor(it)
	if err != nil {
		return err
	}
	return fn(ob)
}
