package brutalinks

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"

	log "git.sr.ht/~mariusor/lw"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/errors"
)

const (
	activities = vocab.CollectionPath("activities")
	actors     = vocab.CollectionPath("actors")
	objects    = vocab.CollectionPath("objects")
)

type fedbox struct {
	baseURL       vocab.IRI
	skipTLSVerify bool
	pub           *vocab.Actor
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
		_, err := url.ParseRequestURI(s)
		if err != nil {
			return err
		}
		f.baseURL = vocab.IRI(s)
		return nil
	}
}

func c2sSign(a *Account, req *http.Request) error {
	if a.Metadata.OAuth.Token == nil {
		return errors.Newf("local account is missing authentication token")
	}
	a.Metadata.OAuth.Token.SetAuthHeader(req)
	return nil
}

func sameHostForAccountAndURL(a *Account, u *url.URL) bool {
	au, _ := url.ParseRequestURI(a.Metadata.ID)
	return au.Hostname() == u.Hostname()
}

func withAccount(a *Account) client.RequestSignFn {
	if !a.IsValid() || !a.IsLogged() || !a.IsLocal() {
		return func(req *http.Request) error { return nil }
	}
	return func(req *http.Request) error {
		if !sameHostForAccountAndURL(a, req.URL) {
			Instance.Logger.WithContext(log.Ctx{
				"url":       req.URL.String(),
				"account":   a.Handle,
				"accountID": a.Metadata.ID,
			}).Warnf("trying to sign S2S request from client")
			return nil
		}
		return c2sSign(a, req)
	}
}

func (f *fedbox) SignBy(signer *Account) {
	if signer == nil {
		return
	}

	f.client.SignFn(withAccount(signer))
}

func SkipTLSCheck(skip bool) OptionFn {
	return func(f *fedbox) error {
		f.skipTLSVerify = skip
		return nil
	}
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
		client.SkipTLSValidation(f.skipTLSVerify),
		client.SetDefaultHTTPClient(),
	)
	service, err := f.client.LoadIRI(f.baseURL)
	if err != nil {
		return &f, err
	}
	return &f, vocab.OnActor(service, func(a *vocab.Actor) error {
		f.pub = a
		return nil
	})
}

func (f fedbox) normaliseIRI(i vocab.IRI) vocab.IRI {
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
	if iu.Path != "" {
		iu.Path = path.Clean(iu.Path)
	}
	return vocab.IRI(iu.String())
}

func (f fedbox) collection(ctx context.Context, i vocab.IRI) (vocab.CollectionInterface, error) {
	it, err := f.client.CtxLoadIRI(ctx, i)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load IRI: %s", i)
	}
	if it == nil {
		return nil, errors.Newf("Unable to load IRI, nil item: %s", i)
	}
	typ := it.GetType()
	if !vocab.CollectionTypes.Contains(it.GetType()) {
		return nil, errors.Errorf("Response item type is not a valid collection: %s", typ)
	}

	if !vocab.CollectionTypes.Contains(typ) {
		return nil, errors.Errorf("Unable to convert item type %s to any of the collection types", typ)
	}
	return it.(vocab.CollectionInterface), nil
}

func (f fedbox) object(ctx context.Context, i vocab.IRI) (vocab.Item, error) {
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

type LoadFn func(vocab.Item, ...client.FilterFn) vocab.IRI

func iri(i vocab.IRI, f ...client.FilterFn) vocab.IRI {
	return vocab.IRI(fmt.Sprintf("%s%s", i, rawFilterQuery(f...)))
}

func inbox(a vocab.Item, f ...client.FilterFn) vocab.IRI {
	return iri(vocab.Inbox.IRI(a), f...)
}

func outbox(a vocab.Item, f ...client.FilterFn) vocab.IRI {
	return iri(vocab.Outbox.IRI(a), f...)
}

func following(a vocab.Item, f ...client.FilterFn) vocab.IRI {
	return iri(vocab.Following.IRI(a), f...)
}

func followers(a vocab.Item, f ...client.FilterFn) vocab.IRI {
	return iri(vocab.Followers.IRI(a), f...)
}

func liked(a vocab.Item, f ...client.FilterFn) vocab.IRI {
	return iri(vocab.Liked.IRI(a), f...)
}

func likes(o vocab.Item, f ...client.FilterFn) vocab.IRI {
	return iri(vocab.Likes.IRI(o), f...)
}

func shares(o vocab.Item, f ...client.FilterFn) vocab.IRI {
	return iri(vocab.Shares.IRI(o), f...)
}

func replies(o vocab.Item, f ...client.FilterFn) vocab.IRI {
	return iri(vocab.Replies.IRI(o), f...)
}

func blocked(a vocab.Item, f ...client.FilterFn) vocab.IRI {
	return iri(vocab.CollectionPath("blocked").IRI(a), f...)
}

func ignored(a vocab.Item, f ...client.FilterFn) vocab.IRI {
	return iri(vocab.CollectionPath("ignored").IRI(a), f...)
}

func validateActor(a vocab.Item) error {
	if a == nil {
		return errors.Errorf("Actor is nil")
	}
	if a.IsObject() && !vocab.ActorTypes.Contains(a.GetType()) {
		return errors.Errorf("Invalid Actor type %s", a.GetType())
	}
	return nil
}

func validateObject(o vocab.Item) error {
	if o == nil {
		return errors.Errorf("object is nil")
	}
	if o.IsObject() && !vocab.ObjectTypes.Contains(o.GetType()) {
		return errors.Errorf("invalid Object type %q", o.GetType())
	}
	return nil
}

type CollectionFn func(context.Context, *Filters) (vocab.CollectionInterface, error)

func (f fedbox) Inbox(ctx context.Context, actor vocab.Item, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(ctx, inbox(actor, filters...))
}

func (f fedbox) Outbox(ctx context.Context, actor vocab.Item, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(ctx, outbox(actor, filters...))
}

func (f fedbox) Following(ctx context.Context, actor vocab.Item, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(ctx, following(actor, filters...))
}

func (f fedbox) Followers(ctx context.Context, actor vocab.Item, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(ctx, followers(actor, filters...))
}

func (f fedbox) Likes(ctx context.Context, object vocab.Item, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(ctx, likes(object, filters...))
}

func (f fedbox) Liked(ctx context.Context, actor vocab.Item, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return f.collection(ctx, liked(actor, filters...))
}

func (f fedbox) Replies(ctx context.Context, object vocab.Item, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(ctx, replies(object, filters...))
}

func (f fedbox) Shares(ctx context.Context, object vocab.Item, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	if err := validateObject(object); err != nil {
		return nil, err
	}
	return f.collection(ctx, shares(object, filters...))
}

func (f fedbox) Collection(ctx context.Context, i vocab.IRI, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	return f.collection(ctx, iri(i, filters...))
}

func (f fedbox) Actor(ctx context.Context, iri vocab.IRI) (*vocab.Actor, error) {
	it, err := f.object(ctx, iri)
	if err != nil {
		return anonymousActor, errors.Annotatef(err, "Unable to load Actor: %s", iri)
	}
	return vocab.ToActor(it)
}

func (f fedbox) Activity(ctx context.Context, iri vocab.IRI) (*vocab.Activity, error) {
	it, err := f.object(ctx, iri)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load Activity: %s", iri)
	}
	return vocab.ToActivity(it)
}

func (f fedbox) Object(ctx context.Context, iri vocab.IRI) (*vocab.Object, error) {
	it, err := f.object(ctx, iri)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load Object: %s", iri)
	}
	return vocab.ToObject(it)
}

func (f fedbox) Activities(ctx context.Context, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	return f.collection(ctx, iri(activities.IRI(f.Service()), filters...))
}

func (f fedbox) Actors(ctx context.Context, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	return f.collection(ctx, iri(actors.IRI(f.Service()), filters...))
}

func (f fedbox) Objects(ctx context.Context, filters ...client.FilterFn) (vocab.CollectionInterface, error) {
	return f.collection(ctx, iri(objects.IRI(f.Service()), filters...))
}

func validateIRIForRequest(i vocab.IRI) error {
	u, err := i.URL()
	if err != nil {
		return err
	}
	if u.Host == "" {
		return errors.Newf("Host is empty")
	}
	return nil
}

func (f fedbox) ToOutbox(ctx context.Context, a vocab.Item) (vocab.IRI, vocab.Item, error) {
	iri := vocab.IRI("")
	vocab.OnActivity(a, func(a *vocab.Activity) error {
		iri = outbox(a.Actor)
		return nil
	})
	if err := validateIRIForRequest(iri); err != nil {
		return "", nil, errors.Annotatef(err, "Invalid Outbox IRI")
	}
	return f.client.CtxToCollection(ctx, f.normaliseIRI(iri), a)
}

func (f fedbox) ToInbox(ctx context.Context, a vocab.Item) (vocab.IRI, vocab.Item, error) {
	iri := vocab.IRI("")
	vocab.OnActivity(a, func(a *vocab.Activity) error {
		iri = inbox(a.Actor)
		return nil
	})
	if err := validateIRIForRequest(iri); err != nil {
		return "", nil, errors.Annotatef(err, "Invalid Inbox IRI")
	}
	return f.client.CtxToCollection(ctx, f.normaliseIRI(iri), a)
}

func (f *fedbox) Service() *vocab.Service {
	if f.pub == nil {
		return &vocab.Actor{ID: f.baseURL, Type: vocab.ServiceType}
	}
	return f.pub
}
