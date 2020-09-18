package app

import (
	"context"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/errors"
	"github.com/go-ap/handlers"
	"github.com/mariusor/littr.go/internal/log"
	"net/url"
	"strings"
)

const (
	activities = handlers.CollectionType("activities")
	actors     = handlers.CollectionType("actors")
	objects    = handlers.CollectionType("objects")
)

type fedbox struct {
	baseURL *url.URL
	client  client.ActivityPub
	infoFn  CtxLogFn
	errFn   CtxLogFn
}

type OptionFn func(*fedbox) error

func SetInfoLogger(logFn CtxLogFn) OptionFn {
	return func(f *fedbox) error {
		if logFn == nil {
			f.infoFn = logFn
		}
		return nil
	}
}
func SetErrorLogger(errFn CtxLogFn) OptionFn {
	return func(f *fedbox) error {
		if errFn != nil {
			f.errFn = errFn
		}
		return nil
	}
}
func SetURL(s string) OptionFn {
	return func(f *fedbox) error {
		u, err := url.Parse(s)
		if err != nil {
			return err
		}
		f.baseURL = u
		return nil
	}
}

func SetSignFn(signer client.RequestSignFn) OptionFn {
	return func(f *fedbox) error {
		f.client.SignFn(signer)
		return nil
	}
}

func (f *fedbox) SignFn(signer client.RequestSignFn) {
	if signer == nil {
		return
	}
	f.client.SignFn(signer)
}

func SetUA(s string) OptionFn {
	return func(f *fedbox) error {
		client.UserAgent = s
		return nil
	}
}

var optionLogFn = func(fn CtxLogFn) func(ctx ...client.Ctx) client.LogFn {
	return func(ctx ...client.Ctx) client.LogFn {
		c := make([]log.Ctx, 0)
		for _, v := range ctx {
			c = append(c, log.Ctx(v))
		}
		return client.LogFn(fn(c...))
	}
}

func NewClient(o ...OptionFn) (*fedbox, error) {
	f := fedbox{
		infoFn: defaultCtxLogFn,
		errFn:  defaultCtxLogFn,
	}
	for _, fn := range o {
		if err := fn(&f); err != nil {
			return nil, err
		}
	}

	f.client = client.New(
		client.SetErrorLogger(optionLogFn(f.errFn)),
		client.SetInfoLogger(optionLogFn(f.infoFn)),
	)
	return &f, nil
}

func (f fedbox) normaliseIRI(i pub.IRI) pub.IRI {
	if u, e := i.URL(); e == nil && u.Scheme != f.baseURL.Scheme {
		return pub.IRI(strings.Replace(i.String(), u.Scheme, f.baseURL.Scheme, 1))
	}
	return i
}

func (f fedbox) collection(i pub.IRI) (pub.CollectionInterface, error) {
	i = f.normaliseIRI(i)
	it, err := f.client.CtxLoadIRI(context.Background(), i)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load IRI: %s", i)
	}
	var col pub.CollectionInterface
	typ := it.GetType()
	if !pub.CollectionTypes.Contains(it.GetType()) {
		return nil, errors.Errorf("Response item type is not a valid collection: %s", typ)
	}
	var ok bool
	switch typ {
	case pub.CollectionType:
		col, ok = it.(*pub.Collection)
	case pub.CollectionPageType:
		col, ok = it.(*pub.CollectionPage)
	case pub.OrderedCollectionType:
		col, ok = it.(*pub.OrderedCollection)
	case pub.OrderedCollectionPageType:
		col, ok = it.(*pub.OrderedCollectionPage)
	}
	if !ok {
		return nil, errors.Errorf("Unable to convert item type %s to any of the collection types", typ)
	}
	return col, nil
}

func (f fedbox) object(i pub.IRI) (pub.Item, error) {
	return f.client.CtxLoadIRI(context.Background(), i)
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
func iri(i pub.IRI, f ...FilterFn) pub.IRI {
	return pub.IRI(fmt.Sprintf("%s%s", i, rawFilterQuery(f...)))
}
func inbox(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(handlers.Inbox.IRI(a), f...)
}
func outbox(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(handlers.Outbox.IRI(a), f...)
}
func following(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(handlers.Following.IRI(a), f...)
}
func followers(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(handlers.Followers.IRI(a), f...)
}
func liked(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(handlers.Liked.IRI(a), f...)
}
func likes(o pub.Item, f ...FilterFn) pub.IRI {
	return iri(handlers.Likes.IRI(o), f...)
}
func shares(o pub.Item, f ...FilterFn) pub.IRI {
	return iri(handlers.Shares.IRI(o), f...)
}
func replies(o pub.Item, f ...FilterFn) pub.IRI {
	return iri(handlers.Replies.IRI(o), f...)
}
func validateActor(a pub.Item) error {
	if a == nil {
		return errors.Errorf("Actor is nil")
	}
	if a.IsObject() && !pub.ActorTypes.Contains(a.GetType()) {
		return errors.Errorf("Invalid Actor type %s", a.GetType())
	}
	return nil
}
func validateObject(o pub.Item) error {
	if o == nil {
		return errors.Errorf("object is nil")
	}
	if o.IsObject() && !pub.ObjectTypes.Contains(o.GetType()) {
		return errors.Errorf("invalid Object type %q", o.GetType())
	}
	return nil
}

type FilterFn func() url.Values

type CollectionFn func() (pub.CollectionInterface, error)

func (f fedbox) Inbox(actor pub.Item, filters ...FilterFn) (pub.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(inbox(actor, filters...))
}
func (f fedbox) Outbox(actor pub.Item, filters ...FilterFn) (pub.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(outbox(actor, filters...))
}
func (f fedbox) Following(actor pub.Item, filters ...FilterFn) (pub.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(following(actor, filters...))
}
func (f fedbox) Followers(actor pub.Item, filters ...FilterFn) (pub.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(followers(actor, filters...))
}
func (f fedbox) Likes(object pub.Item, filters ...FilterFn) (pub.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(likes(object, filters...))
}
func (f fedbox) Liked(actor pub.Item, filters ...FilterFn) (pub.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(liked(actor, filters...))
}
func (f fedbox) Replies(object pub.Item, filters ...FilterFn) (pub.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(replies(object, filters...))
}
func (f fedbox) Shares(object pub.Item, filters ...FilterFn) (pub.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(shares(object, filters...))
}

func (f fedbox) Collection(i pub.IRI, filters ...FilterFn) (pub.CollectionInterface, error) {
	return f.collection(iri(i, filters...))
}

func (f fedbox) Actor(iri pub.IRI) (*pub.Actor, error) {
	it, err := f.object(iri)
	if err != nil {
		return anonymousActor, errors.Annotatef(err, "Unable to load Actor: %s", iri)
	}
	var person *pub.Actor
	pub.OnActor(it, func(p *pub.Actor) error {
		person = p
		return nil
	})
	return person, nil
}

func (f fedbox) Activity(iri pub.IRI) (*pub.Activity, error) {
	it, err := f.object(iri)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load Activity: %s", iri)
	}
	var activity *pub.Activity
	pub.OnActivity(it, func(a *pub.Activity) error {
		activity = a
		return nil
	})
	return activity, nil
}

func (f fedbox) Object(iri pub.IRI) (*pub.Object, error) {
	it, err := f.object(iri)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load Object: %s", iri)
	}
	var object *pub.Object
	pub.OnObject(it, func(o *pub.Object) error {
		object = o
		return nil
	})
	return object, nil
}

func (f fedbox) Activities(filters ...FilterFn) (pub.CollectionInterface, error) {
	return f.collection(iri(activities.IRI(f.Service()), filters...))
}

func (f fedbox) Actors(filters ...FilterFn) (pub.CollectionInterface, error) {
	return f.collection(iri(actors.IRI(f.Service()), filters...))
}

func (f fedbox) Objects(filters ...FilterFn) (pub.CollectionInterface, error) {
	return f.collection(iri(objects.IRI(f.Service()), filters...))
}

func validateIRIForRequest(i pub.IRI) error {
	u, err := i.URL()
	if err != nil {
		return err
	}
	if u.Host == "" {
		return errors.Newf("Host is empty")
	}
	return nil
}

func (f fedbox) req(iri pub.IRI, a pub.Item) (pub.IRI, pub.Item, error) {
	if err := validateIRIForRequest(iri); err != nil {
		return "", nil, errors.Annotatef(err, "Invalid IRI to post to")
	}
	return f.client.CtxToCollection(context.Background(), iri, a)
}

func (f fedbox) ToOutbox(a pub.Item) (pub.IRI, pub.Item, error) {
	iri := pub.IRI("")
	pub.OnActivity(a, func(a *pub.Activity) error {
		iri = outbox(a.Actor)
		return nil
	})
	return f.req(iri, a)
}

func (f fedbox) ToInbox(a pub.Item) (pub.IRI, pub.Item, error) {
	iri := pub.IRI("")
	pub.OnActivity(a, func(a *pub.Activity) error {
		iri = inbox(a.Actor)
		return nil
	})
	return f.req(iri, a)
}

func (f *fedbox) Service() *pub.Service {
	iri := strings.TrimRight(f.baseURL.String(), "/")
	s := &pub.Service{
		ID:   pub.ID(iri),
		Type: pub.ServiceType,
		URL:  pub.IRI(iri),
	}
	s.Inbox = inbox(s)

	return s
}
