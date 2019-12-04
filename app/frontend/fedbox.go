package frontend

import (
	"fmt"
	ap "github.com/go-ap/activitypub"
	"github.com/go-ap/activitypub/client"
	"github.com/go-ap/errors"
	"github.com/go-ap/handlers"
	"net/url"
)

type FedboxConfig struct {
	URL string
	LogFn client.LogFn
	ErrFn client.LogFn
	SignFn client.RequestSignFn
}

type fedbox struct {
	baseURL *url.URL
	client  client.HttpClient
	logFn client.LogFn
}

func Fedbox(config FedboxConfig) (*fedbox, error) {
	u, err := url.Parse(config.URL)
	if err != nil {
		return nil, err
	}

	client.InfoLogger = config.LogFn
	client.ErrorLogger = config.ErrFn

	c := client.NewClient()
	c.SignFn(config.SignFn)
	return &fedbox{
		baseURL: u,
		client: c,
	}, nil
}

func (f fedbox) collection(i ap.IRI) (ap.CollectionInterface, error) {
	it, err := f.client.LoadIRI(i)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load IRI: %s", i)
	}
	var col ap.CollectionInterface
	typ := it.GetType()
	if !ap.CollectionTypes.Contains(it.GetType()) {
		return nil, errors.Errorf("Response item type is not a valid collection: %s", typ)
	}
	var ok bool
	switch typ {
	case ap.CollectionType:
		col, ok = it.(*ap.Collection)
	case ap.CollectionPageType:
		col, ok = it.(*ap.CollectionPage)
	case ap.OrderedCollectionType:
		col, ok = it.(*ap.OrderedCollection)
	case ap.OrderedCollectionPageType:
		 col, ok = it.(*ap.OrderedCollectionPage)
	}
	if !ok {
		return nil, errors.Errorf("Unable to convert item type %s to any of the collection types", typ)
	}
	return col, nil
}

func (f fedbox) object(i ap.IRI) (ap.Item, error) {
	return f.client.LoadIRI(i)
}
func rawFilterQuery(f ...FilterFn) string {
	if len(f) == 0 {
		return ""
	}
	q := make(url.Values)
	for _, ff := range f {
		qq := ff()
		for k, v := range qq {
			q[k] = append(q[k], v...)
		}
	}
	if len(q) == 0 {
		return ""
	}
	
	return "?" + q.Encode()
}
func iri(i ap.Item, col handlers.CollectionType, f ...FilterFn) ap.IRI {
	return ap.IRI(fmt.Sprintf("%s/%s%s", i.GetLink(), col, rawFilterQuery(f...)))
}
func inbox(a ap.Item, f ...FilterFn) ap.IRI {
	return iri(a, handlers.Inbox, f...)
}
func outbox(a ap.Item, f ...FilterFn) ap.IRI {
	return iri(a, handlers.Outbox, f...)
}
func following(a ap.Item, f ...FilterFn) ap.IRI {
	return iri(a, handlers.Following, f...)
}
func followers(a ap.Item, f ...FilterFn) ap.IRI {
	return iri(a, handlers.Followers, f...)
}
func likes(a ap.Item, f ...FilterFn) ap.IRI {
	return iri(a, handlers.Likes, f...)
}
func shares(a ap.Item, f ...FilterFn) ap.IRI {
	return iri(a, handlers.Shares, f...)
}
func replies(a ap.Item, f ...FilterFn) ap.IRI {
	return iri(a, handlers.Replies, f...)
}
func liked(a ap.Item, f ...FilterFn) ap.IRI {
	return iri(a, handlers.Liked, f...)
}
func validateActor(actor ap.Item) error {
	if actor == nil {
		return errors.Errorf("Actor is nil")
	}
	if !ap.ActorTypes.Contains(actor.GetType()) {
		return errors.Errorf("Invalid Actor type %s", actor.GetType())
	}
	return nil
}
func validateObject(object ap.Item) error {
	if object == nil {
		return errors.Errorf("Object is nil")
	}
	if !ap.ObjectTypes.Contains(object.GetType()) {
		return errors.Errorf("Invalid Object type %s", object.GetType())
	}
	return nil
}

type FilterFn func() url.Values

func (f fedbox) Inbox(actor ap.Item, filters ...FilterFn) (ap.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(inbox(actor, filters...))
}
func (f fedbox) Outbox(actor ap.Item, filters ...FilterFn) (ap.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(outbox(actor, filters...))
}
func (f fedbox) Following(actor ap.Item, filters ...FilterFn) (ap.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(following(actor, filters...))
}
func (f fedbox) Followers(actor ap.Item, filters ...FilterFn) (ap.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(followers(actor, filters...))
}
func (f fedbox) Likes(actor ap.Item, filters ...FilterFn) (ap.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(likes(actor, filters...))
}
func (f fedbox) Liked(object ap.Item, filters ...FilterFn) (ap.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(liked(object, filters...))
}
func (f fedbox) Replies(object ap.Item, filters ...FilterFn) (ap.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(replies(object, filters...))
}
func (f fedbox) Shares(object ap.Item, filters ...FilterFn) (ap.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(shares(object, filters...))
}

func (f fedbox) Actor(iri ap.IRI) (*ap.Actor, error) {
	it, err := f.object(iri)
	if err != nil {
		return anonymousActor(), errors.Annotatef(err, "Unable to load Actor: %s", iri)
	}
	var person *ap.Actor
	err = ap.OnActor(it, func(p *ap.Actor) error {
		person = p
		return nil
	})
	return person, err
}

func (f fedbox) Activity(iri ap.IRI) (*ap.Activity, error) {
	it, err := f.object(iri)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load Activity: %s", iri)
	}
	var activity *ap.Activity
	err = ap.OnActivity(it, func(a *ap.Activity) error {
		activity = a
		return nil
	})
	return activity, err
}

func (f fedbox) Object(iri ap.IRI) (*ap.Object, error) {
	it, err := f.object(iri)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load Object: %s", iri)
	}
	var object *ap.Object
	err = ap.OnObject(it, func(o *ap.Object) error {
		object = o
		return nil
	})
	return object, err
}
