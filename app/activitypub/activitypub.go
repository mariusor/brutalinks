package activitypub

import (
	"github.com/buger/jsonparser"
	ap "github.com/go-ap/activitypub"
	as "github.com/go-ap/activitystreams"
	"github.com/go-ap/auth"
	"github.com/go-ap/jsonld"
)

// Person it should be identical to:
//    github.com/go-ap/activitypub/actors.go#Actor
// We need it here in order to be able to add to it our Score property
type Person struct {
	auth.Person
	// Score is our own custom property for which we needed to extend the existing AP one
	Score int64 `jsonld:"score"`
}

// Article it should be identical to:
//    github.com/go-ap/activitypub/objects.go#Object
// We need it here in order to be able to add to it our Score property
type Article struct {
	ap.Object
	Score int64 `jsonld:"score"`
}

// GetID returns the ObjectID pointer of current Person instance
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

// GetID returns the ObjectID pointer of current Article instance
func (a Article) GetID() *as.ObjectID {
	id := as.ObjectID(a.ID)
	return &id
}

// GetLink returns the IRI of the Article object
func (a Article) GetLink() as.IRI {
	return as.IRI(a.ID)
}

// GetType returns the current Article object's type
func (a Article) GetType() as.ActivityVocabularyType {
	return as.ActivityVocabularyType(a.Type)
}

// IsLink returns false for an Article object
func (a Article) IsLink() bool {
	return false
}

// IsObject returns true for an Article object
func (a Article) IsObject() bool {
	return true
}

// UnmarshalJSON tries to load json data to Article object
func (a *Article) UnmarshalJSON(data []byte) error {
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

// UnmarshalJSON tries to load json data to Person object
func (p *Person) UnmarshalJSON(data []byte) error {
	pers := auth.Person{}
	if err := pers.UnmarshalJSON(data); err != nil {
		return err
	}

	p.Person = pers
	if score, err := jsonparser.GetInt(data, "score"); err == nil {
		p.Score = score
	}

	return nil
}

// UnmarshalJSON tries to detect the type of the object in the json data and then outputs a matching
// ActivityStreams object, if possible
func UnmarshalJSON(data []byte) (as.Item, error) {
	i, err := as.UnmarshalJSON(data)
	switch i.GetType() {
	case as.PersonType:
		i = Person{}
		jsonld.Unmarshal(data, &i)
	case as.ArticleType:
		i = Article{}
		jsonld.Unmarshal(data, &i)
	}
	return i, err
}

func JSONGetItemByType(typ as.ActivityVocabularyType) (as.Item, error) {
	var ret as.Item
	var err error

	switch typ {
	case as.DocumentType:
		fallthrough
	case as.NoteType:
		fallthrough
	case as.PageType:
		fallthrough
	case as.ArticleType:
		ret = &Article{}
		o := ret.(*Article)
		o.Type = typ
	case as.ApplicationType:
		fallthrough
	case as.GroupType:
		fallthrough
	case as.OrganizationType:
		fallthrough
	case as.ServiceType:
		fallthrough
	case as.ActorType:
		fallthrough
	case as.PersonType:
		ret = &Person{}
		o := ret.(*Person)
		o.Type = typ
	default:
		return as.JSONGetItemByType(typ)
	}
	return ret, err
}

