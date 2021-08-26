package app

import (
	"context"
	"crypto"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"path"

	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/errors"
	"github.com/go-ap/handlers"
	"github.com/mariusor/go-littr/internal/log"
)

const (
	activities = handlers.CollectionType("activities")
	actors     = handlers.CollectionType("actors")
	objects    = handlers.CollectionType("objects")
)

type fedbox struct {
	baseURL       pub.IRI
	skipTLSVerify bool
	pub           *pub.Actor
	client        *client.C
	infoFn        CtxLogFn
	errFn         CtxLogFn
}

type OptionFn func(*fedbox) error

func SetInfoLogger(logFn CtxLogFn) OptionFn {
	return func(f *fedbox) error {
		if logFn != nil {
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
		_, err := url.Parse(s)
		if err != nil {
			return err
		}
		f.baseURL = pub.IRI(s)
		return nil
	}
}

func withAccountC2S(a *Account) (client.RequestSignFn, error) {
	if !a.IsValid() || !a.IsLogged() {
		return nil, errors.Newf("invalid local account")
	}
	if a.Metadata.OAuth.Token == nil {
		return nil, errors.Newf("invalid local account")
	}
	return func(req *http.Request) error {
		// TODO(marius): this needs to be added to the federated requests, which we currently don't support
		a.Metadata.OAuth.Token.SetAuthHeader(req)
		return nil
	}, nil
}

func withAccountS2S(a *Account) (client.RequestSignFn, error) {
	// TODO(marius): this needs to be added to the federated requests, which we currently don't support
	if !a.IsValid() || !a.IsLogged() {
		return nil, nil
	}
	k := a.Metadata.Key
	if k == nil {
		return nil, nil
	}
	var prv crypto.PrivateKey
	var err error
	if k.ID == "id-rsa" {
		prv, err = x509.ParsePKCS8PrivateKey(k.Private)
	}
	if err != nil {
		return nil, err
	}
	if k.ID == "id-ecdsa" {
		prv, err = x509.ParseECPrivateKey(k.Private)
		if err != nil {
			return nil, err
		}
	}
	return getSigner(k.ID, prv).Sign, nil
}

func SetSignFn(signer *Account) OptionFn {
	return func(f *fedbox) error {
		f.SignBy(signer)
		return nil
	}
}

func (f *fedbox) SignBy(signer *Account) {
	if signer == nil {
		return
	}
	signFn, err := withAccountC2S(signer)
	if err != nil {
		f.errFn()(err.Error())
	}
	f.client.SignFn(signFn)
}

func SetUA(s string) OptionFn {
	return func(f *fedbox) error {
		client.UserAgent = s
		return nil
	}
}

func SkipTLSCheck(skip bool) OptionFn {
	return func(f *fedbox) error {
		f.skipTLSVerify = skip
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
		client.SkipTLSValidation(f.skipTLSVerify),
		client.SetDefaultHTTPClient(),
	)
	service, err := f.client.LoadIRI(f.baseURL)
	if err != nil {
		return &f, err
	}
	return &f, pub.OnActor(service, func(a *pub.Actor) error {
		f.pub = a
		return nil
	})
}

func (f fedbox) normaliseIRI(i pub.IRI) pub.IRI {
	iu, ie := i.URL()
	if i.Contains(f.baseURL, false) {
		bu, be := f.baseURL.URL()
		if ie != nil || be != nil {
			return i
		}
		iu.Host = bu.Host
		if iu.Scheme != bu.Scheme {
			iu.Scheme = bu.Scheme
		}
	}
	iu.Path = path.Clean(iu.Path)
	return pub.IRI(iu.String())
}

func (f fedbox) collection(ctx context.Context, i pub.IRI) (pub.CollectionInterface, error) {
	it, err := f.client.CtxLoadIRI(ctx, f.normaliseIRI(i))
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load IRI: %s", i)
	}
	if it == nil {
		return nil, errors.Newf("Unable to load IRI, nil item: %s", i)
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

func (f fedbox) object(ctx context.Context, i pub.IRI) (pub.Item, error) {
	return f.client.CtxLoadIRI(ctx, f.normaliseIRI(i))
}

func rawFilterQuery(f ...client.FilterFn) string {
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

type LoadFn func(pub.Item, ...client.FilterFn) pub.IRI

func iri(i pub.IRI, f ...client.FilterFn) pub.IRI {
	return pub.IRI(fmt.Sprintf("%s%s", i, rawFilterQuery(f...)))
}

func inbox(a pub.Item, f ...client.FilterFn) pub.IRI {
	return iri(handlers.Inbox.IRI(a), f...)
}

func outbox(a pub.Item, f ...client.FilterFn) pub.IRI {
	return iri(handlers.Outbox.IRI(a), f...)
}

func following(a pub.Item, f ...client.FilterFn) pub.IRI {
	return iri(handlers.Following.IRI(a), f...)
}

func followers(a pub.Item, f ...client.FilterFn) pub.IRI {
	return iri(handlers.Followers.IRI(a), f...)
}

func liked(a pub.Item, f ...client.FilterFn) pub.IRI {
	return iri(handlers.Liked.IRI(a), f...)
}

func likes(o pub.Item, f ...client.FilterFn) pub.IRI {
	return iri(handlers.Likes.IRI(o), f...)
}

func shares(o pub.Item, f ...client.FilterFn) pub.IRI {
	return iri(handlers.Shares.IRI(o), f...)
}

func replies(o pub.Item, f ...client.FilterFn) pub.IRI {
	return iri(handlers.Replies.IRI(o), f...)
}

func blocked(a pub.Item, f ...client.FilterFn) pub.IRI {
	return iri(handlers.CollectionType("blocked").IRI(a), f...)
}

func ignored(a pub.Item, f ...client.FilterFn) pub.IRI {
	return iri(handlers.CollectionType("ignored").IRI(a), f...)
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

type CollectionFn func(context.Context, *Filters) (pub.CollectionInterface, error)

func (f fedbox) Inbox(ctx context.Context, actor pub.Item, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(ctx, inbox(actor, filters...))
}

func (f fedbox) Outbox(ctx context.Context, actor pub.Item, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(ctx, outbox(actor, filters...))
}

func (f fedbox) Following(ctx context.Context, actor pub.Item, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(ctx, following(actor, filters...))
}

func (f fedbox) Followers(ctx context.Context, actor pub.Item, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(ctx, followers(actor, filters...))
}

func (f fedbox) Likes(ctx context.Context, object pub.Item, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(ctx, likes(object, filters...))
}

func (f fedbox) Liked(ctx context.Context, actor pub.Item, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(ctx, liked(actor, filters...))
}

func (f fedbox) Replies(ctx context.Context, object pub.Item, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(ctx, replies(object, filters...))
}

func (f fedbox) Shares(ctx context.Context, object pub.Item, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(ctx, shares(object, filters...))
}

func (f fedbox) Collection(ctx context.Context, i pub.IRI, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	return f.collection(ctx, iri(i, filters...))
}

func (f fedbox) Actor(ctx context.Context, iri pub.IRI) (*pub.Actor, error) {
	it, err := f.object(ctx, iri)
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

func (f fedbox) Activity(ctx context.Context, iri pub.IRI) (*pub.Activity, error) {
	it, err := f.object(ctx, iri)
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

func (f fedbox) Object(ctx context.Context, iri pub.IRI) (*pub.Object, error) {
	it, err := f.object(ctx, iri)
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

func (f fedbox) Activities(ctx context.Context, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	return f.collection(ctx, iri(activities.IRI(f.Service()), filters...))
}

func (f fedbox) Actors(ctx context.Context, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	return f.collection(ctx, iri(actors.IRI(f.Service()), filters...))
}

func (f fedbox) Objects(ctx context.Context, filters ...client.FilterFn) (pub.CollectionInterface, error) {
	return f.collection(ctx, iri(objects.IRI(f.Service()), filters...))
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

func (f fedbox) ToOutbox(ctx context.Context, a pub.Item) (pub.IRI, pub.Item, error) {
	iri := pub.IRI("")
	pub.OnActivity(a, func(a *pub.Activity) error {
		iri = outbox(a.Actor)
		return nil
	})
	if err := validateIRIForRequest(iri); err != nil {
		return "", nil, errors.Annotatef(err, "Invalid Outbox IRI")
	}
	return f.client.CtxToCollection(ctx, f.normaliseIRI(iri), a)
}

func (f fedbox) ToInbox(ctx context.Context, a pub.Item) (pub.IRI, pub.Item, error) {
	iri := pub.IRI("")
	pub.OnActivity(a, func(a *pub.Activity) error {
		iri = inbox(a.Actor)
		return nil
	})
	if err := validateIRIForRequest(iri); err != nil {
		return "", nil, errors.Annotatef(err, "Invalid Inbox IRI")
	}
	return f.client.CtxToCollection(ctx, f.normaliseIRI(iri), a)
}

func (f *fedbox) Service() *pub.Service {
	if f.pub == nil {
		return &pub.Actor{ ID: f.baseURL, Type: pub.ServiceType }
	}
	return f.pub
}
