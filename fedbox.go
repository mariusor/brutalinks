package brutalinks

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"

	"git.sr.ht/~mariusor/brutalinks/internal/config"
	"git.sr.ht/~mariusor/cache"
	log "git.sr.ht/~mariusor/lw"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/client/credentials"
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
	transport     http.RoundTripper
	pub           *vocab.Actor
	client        client.C
	l             log.Logger
	infoFn        CtxLogFn
	errFn         CtxLogFn
}

type OptionFn func(*fedbox) error

func WithLogger(l log.Logger) OptionFn {
	return func(f *fedbox) error {
		f.l = l
		if l != nil {
			f.infoFn = func(ctx ...log.Ctx) LogFn {
				return l.WithContext(ctx...).Debugf
			}
			f.errFn = func(ctx ...log.Ctx) LogFn {
				return l.WithContext(ctx...).Warnf
			}
		}
		return nil
	}
}

func WithURL(s string) OptionFn {
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
	if (a == nil || a.Metadata == nil || a.Metadata.ID == "") && u != nil {
		return false
	}
	au, _ := url.ParseRequestURI(a.Metadata.ID)
	return au.Hostname() == u.Hostname()
}

func (f fedbox) withAccount(a *Account) client.RequestSignFn {
	if !a.IsValid() || !a.IsLogged() || !a.IsLocal() {
		return func(req *http.Request) error { return nil }
	}
	return func(req *http.Request) error {
		if !sameHostForAccountAndURL(a, req.URL) {
			// NOTE(marius): we don't have access to the actor's private key,
			// so we can't sign requests going to other instances
			// If at any point we decide to keep the private key here, we can use it
			// to sign S2S requests:
			//
			// if a.Metadata.PrivateKey != nil {
			//      s2sSign(a.Metadata.PrivateKey, req)
			// }
			f.l.WithContext(log.Ctx{
				"method": req.Method,
				"url":    req.URL.String(),
				"handle": a.Handle,
				"IRI":    a.Metadata.ID,
			}).Debugf("skip signing cross server request")
			return nil
		}
		return c2sSign(a, req)
	}
}

func SkipTLSCheck(skip bool) OptionFn {
	return func(f *fedbox) error {
		f.skipTLSVerify = skip
		return nil
	}
}

func WithUA(s string) OptionFn {
	return func(f *fedbox) error {
		client.UserAgent = s
		return nil
	}
}

func WithHTTPTransport(t http.RoundTripper) OptionFn {
	return func(f *fedbox) error {
		f.transport = t
		return nil
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

	options := make([]client.OptionFn, 0)
	if f.transport != nil {
		cl := http.DefaultClient
		cl.Transport = f.transport
		options = append(options, client.WithHTTPClient(cl))
	}
	options = append(options,
		client.WithLogger(f.l.WithContext(log.Ctx{"log": "client"})),
		client.SkipTLSValidation(f.skipTLSVerify),
		client.SetDefaultHTTPClient(),
	)
	f.client = *client.New(options...)
	service, err := f.client.LoadIRI(f.baseURL)
	if err != nil {
		return &f, err
	}
	return &f, vocab.OnActor(service, func(a *vocab.Actor) error {
		f.pub = a
		return nil
	})
}

func (f *fedbox) normaliseIRI(i vocab.IRI) vocab.IRI {
	iu, ie := i.URL()
	if ie != nil {
		return i
	}
	if i.Contains(f.baseURL, false) {
		bu, be := f.baseURL.URL()
		if be != nil {
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
	if vocab.IsNil(it) {
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

func Client(tr http.RoundTripper, conf config.Configuration, l log.Logger) *client.C {
	if tr == nil {
		tr = &http.Transport{}
	}

	baseClient := &http.Client{
		Transport: cache.Private(tr, cache.FS(filepath.Join(conf.SessionsPath, conf.HostName))),
	}

	ua := fmt.Sprintf("%s/%s (+%s)", conf.HostName, Instance.Version, ProjectURL)
	return client.New(
		client.WithUserAgent(ua),
		client.WithLogger(l.WithContext(log.Ctx{"log": "client"})),
		client.WithHTTPClient(baseClient),
		client.SkipTLSValidation(!conf.Env.IsProd()),
		client.SetDefaultHTTPClient(),
	)
}

func (r *repository) ToOutbox(ctx context.Context, cred credentials.C2S, a vocab.Item) (vocab.IRI, vocab.Item, error) {
	sendTo := vocab.IRI("")
	_ = vocab.OnActivity(a, func(a *vocab.Activity) error {
		sendTo = outbox(a.Actor)
		return nil
	})
	if err := validateIRIForRequest(sendTo); err != nil {
		return "", nil, errors.Annotatef(err, "Invalid Outbox IRI")
	}

	sendTo = r.fedbox.normaliseIRI(sendTo)

	tr := cred.Transport(ctx)
	if trr, ok := r.fedbox.transport.(cache.Transport); ok {
		trr.Base = tr
		tr = trr
	}

	cl := Client(tr, *Instance.Conf, r.fedbox.l)
	i, it, err := cl.CtxToCollection(ctx, sendTo, a)
	if err != nil {
		return i, a, err
	}

	toSave := vocab.ItemCollection{it}
	if a, err = cl.LoadIRI(i); err == nil {
		toSave = append(toSave, a)
	}

	_ = r.b.Save(toSave...)

	return i, it, nil
}

func (f *fedbox) Service() *vocab.Service {
	if f.pub == nil {
		return &vocab.Actor{ID: f.baseURL, Type: vocab.ServiceType}
	}
	return f.pub
}
