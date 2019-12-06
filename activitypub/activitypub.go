package activitypub

import (
	"github.com/buger/jsonparser"
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
)

// Actor it should be identical to:
//    github.com/go-ap/activitypub/actors.go#actor
// We need it here in order to be able to add to it our Score property
type Actor struct {
	pub.Actor
	// Score is our own custom property for which we needed to extend the existing AP one
	Score int `jsonld:"score"`
}

// Object it should be identical to:
//    github.com/go-ap/activitypub/objects.go#Object
// We need it here in order to be able to add to it our Score property
type Object struct {
	pub.Object
	Score int `jsonld:"score"`
}

// GetID returns the ID pointer of current Actor instance
func (a Actor) GetID() pub.ID {
	id := a.ID
	return id
}
func (a Actor) GetType() pub.ActivityVocabularyType {
	return a.Type
}
func (a Actor) GetLink() pub.IRI {
	return pub.IRI(a.ID)
}
func (a Actor) IsLink() bool {
	return false
}

func (a Actor) IsObject() bool {
	return true
}

// GetID returns the ID pointer of current Object instance
func (a Object) GetID() pub.ID {
	return a.ID
}

// GetLink returns the IRI of the Object object
func (a Object) GetLink() pub.IRI {
	return pub.IRI(a.ID)
}

// GetType returns the current Object object's type
func (a Object) GetType() pub.ActivityVocabularyType {
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
	ob := pub.Object{}
	err := ob.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	a.Object = ob
	if score, err := jsonparser.GetInt(data, "score"); err == nil {
		a.Score = int(score)
	}

	return nil
}

// UnmarshalJSON tries to load json data to Actor object
func (a *Actor) UnmarshalJSON(data []byte) error {
	p := pub.Actor{}
	if err := p.UnmarshalJSON(data); err != nil {
		return err
	}

	a.Actor = p
	if score, err := jsonparser.GetInt(data, "score"); err == nil {
		a.Score = int(score)
	}

	return nil
}

// JSONGetItemByType
func JSONGetItemByType(typ pub.ActivityVocabularyType) (pub.Item, error) {
	if pub.ActorTypes.Contains(typ) {
		act := &Actor{}
		act.Type = typ
		return act, nil
	} else if pub.ObjectTypes.Contains(typ) {
		ob := &Object{}
		ob.Type = typ
		return ob, nil
	}
	return pub.JSONGetItemByType(typ)
}

// ToObject
func ToObject(it pub.Item) (*Object, error) {
	switch i := it.(type) {
	case *Object:
		return i, nil
	case Object:
		return &i, nil
	default:
		ob, err := pub.ToObject(it)
		if err != nil {
			return nil, err
		}
		return &Object{
			Object: *ob,
		}, nil
	}
	return nil, errors.Newf("unable to convert object")
}

type withObjectFn func(*Object) error

// OnObject
func OnObject(it pub.Item, fn withObjectFn) error {
	ob, err := ToObject(it)
	if err != nil {
		return err
	}
	return fn(ob)
}

// ToActor
func ToActor(it pub.Item) (*Actor, error) {
	switch i := it.(type) {
	case *Actor:
		return i, nil
	case Actor:
		return &i, nil
	default:
		pers, err := pub.ToActor(it)
		if err != nil {
			return nil, err
		}
		return &Actor{
			Actor: *pers,
		}, nil
	}
	return nil, errors.Newf("unable to convert object")
}

type withPersonFn func(*Actor) error

// OnActor
func OnActor(it pub.Item, fn withPersonFn) error {
	ob, err := ToActor(it)
	if err != nil {
		return err
	}
	return fn(ob)
}
