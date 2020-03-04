package app

import (
	"bytes"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/errors"
	"github.com/go-ap/handlers"
	j "github.com/go-ap/jsonld"
	"github.com/mariusor/littr.go/internal/log"
	"io/ioutil"
	"net/http"
	"net/url"
)

const (
	activities = handlers.CollectionType("activities")
	actors     = handlers.CollectionType("actors")
	objects    = handlers.CollectionType("objects")
)

type fedbox struct {
	baseURL *url.URL
	client  client.HttpClient
	infoFn  LogFn
	errFn   LogFn
}

type OptionFn func(*fedbox) error

func SetInfoLogger(logFn LogFn) OptionFn {
	return func(f *fedbox) error {
		f.infoFn = logFn
		client.InfoLogger = func(el ...interface{}) {
			ctx := log.Ctx{}
			for _, i := range el {
				ctx[fmt.Sprintf("%T", i)] = i
			}
			logFn("", ctx)
		}
		return nil
	}
}
func SetErrorLogger(logFn LogFn) OptionFn {
	return func(f *fedbox) error {
		f.errFn = logFn
		client.ErrorLogger = func(el ...interface{}) {
			ctx := log.Ctx{}
			for _, i := range el {
				ctx[fmt.Sprintf("%T", i)] = i
			}
			logFn("", ctx)
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
	f.client.SignFn(signer)
}

func SetUA(s string) OptionFn {
	return func(f *fedbox) error {
		client.UserAgent = s
		return nil
	}
}

func NewClient(o ...OptionFn) (*fedbox, error) {
	f := fedbox{}
	f.client = client.NewClient()
	for _, fn := range o {
		if err := fn(&f); err != nil {
			return nil, err
		}
	}

	//client.ErrorLogger = f.infoFn
	//client.InfoLogger = f.errFn
	return &f, nil
}

func (f fedbox) collection(i pub.IRI) (pub.CollectionInterface, error) {
	it, err := f.client.LoadIRI(i)
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
func iri(i pub.Item, col handlers.CollectionType, f ...FilterFn) pub.IRI {
	if len(col) == 0 {
		return pub.IRI(fmt.Sprintf("%s%s", i.GetLink(), rawFilterQuery(f...)))
	}
	return pub.IRI(fmt.Sprintf("%s/%s%s", i.GetLink(), col, rawFilterQuery(f...)))
}
func inbox(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(a, handlers.Inbox, f...)
}
func outbox(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(a, handlers.Outbox, f...)
}
func following(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(a, handlers.Following, f...)
}
func followers(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(a, handlers.Followers, f...)
}
func likes(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(a, handlers.Likes, f...)
}
func shares(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(a, handlers.Shares, f...)
}
func replies(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(a, handlers.Replies, f...)
}
func liked(a pub.Item, f ...FilterFn) pub.IRI {
	return iri(a, handlers.Liked, f...)
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
func (f fedbox) Likes(actor pub.Item, filters ...FilterFn) (pub.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(likes(actor, filters...))
}
func (f fedbox) Liked(object pub.Item, filters ...FilterFn) (pub.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(liked(object, filters...))
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
	return f.collection(iri(i, "", filters...))
}

func (f fedbox) Actor(iri pub.IRI) (*pub.Actor, error) {
	it, err := f.object(iri)
	if err != nil {
		return anonymousActor(), errors.Annotatef(err, "Unable to load Actor: %s", iri)
	}
	var person *pub.Actor
	err = pub.OnActor(it, func(p *pub.Actor) error {
		person = p
		return nil
	})
	return person, err
}

func (f fedbox) Activity(iri pub.IRI) (*pub.Activity, error) {
	it, err := f.object(iri)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load Activity: %s", iri)
	}
	var activity *pub.Activity
	err = pub.OnActivity(it, func(a *pub.Activity) error {
		activity = a
		return nil
	})
	return activity, err
}

func (f fedbox) Object(iri pub.IRI) (*pub.Object, error) {
	it, err := f.object(iri)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load Object: %s", iri)
	}
	var object *pub.Object
	err = pub.OnObject(it, func(o *pub.Object) error {
		object = o
		return nil
	})
	return object, err
}

func (f fedbox) Activities(filters ...FilterFn) (pub.CollectionInterface, error) {
	return f.collection(iri(pub.IRI(f.baseURL.String()), activities, filters...))
}

func (f fedbox) Actors(filters ...FilterFn) (pub.CollectionInterface, error) {
	return f.collection(iri(pub.IRI(f.baseURL.String()), actors, filters...))
}

func (f fedbox) Objects(filters ...FilterFn) (pub.CollectionInterface, error) {
	return f.collection(iri(pub.IRI(f.baseURL.String()), objects, filters...))
}

func postRequest(f fedbox, url pub.IRI, a pub.Item) (pub.IRI, pub.Item, error) {
	body, err := j.Marshal(a)
	var resp *http.Response
	var it pub.Item
	var iri pub.IRI
	resp, err = f.client.Post(url.String(), client.ContentTypeActivityJson, bytes.NewReader(body))
	if err != nil {
		return iri, it, err
	}
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		f.errFn(err.Error(), nil)
		return iri, it, err
	}
	if resp.StatusCode != http.StatusGone && resp.StatusCode >= http.StatusBadRequest {
		errs := _errors{}
		if err := j.Unmarshal(body, &errs); err != nil {
			f.errFn(fmt.Sprintf("Unable to unmarshal error response: %s", err.Error()), nil)
		}
		if len(errs.Errors) == 0 {
			return iri, it, errors.Newf("Unknown error")
		}
		err := errs.Errors[0]
		return iri, it, errors.WrapWithStatus(err.Code, nil, err.Message)
	}
	iri = pub.IRI(resp.Header.Get("Location"))
	it, err = pub.UnmarshalJSON(body)
	return iri, it, err
}

func (f fedbox) ToOutbox(a pub.Item) (pub.IRI, pub.Item, error) {
	url := pub.IRI("")
	err := pub.OnActivity(a, func(a *pub.Activity) error {
		url = outbox(a.Actor)
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	if len(url) == 0 {
		return "", nil, errors.Newf("Invalid URL to post to")
	}
	return postRequest(f, url, a)
}

func (f fedbox) ToInbox(a pub.Item) (pub.IRI, pub.Item, error) {
	url := pub.IRI("")
	err := pub.OnActivity(a, func(a *pub.Activity) error {
		url = inbox(a.Actor)
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	if len(url) == 0 {
		return "", nil, errors.Newf("Invalid URL to post to")
	}
	return postRequest(f, url, a)
}

func (f *fedbox) Service() pub.Service {
	s := pub.Service{
		ID:   pub.ID(f.baseURL.String()),
		Type: pub.ServiceType,
		URL:  pub.IRI(f.baseURL.String()),
	}
	s.Inbox = inbox(s)

	return s
}
