package brutalinks

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"git.sr.ht/~mariusor/box"
	"git.sr.ht/~mariusor/cache"
	log "git.sr.ht/~mariusor/lw"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/errors"
	"github.com/go-ap/filters"
)

const (
	activities = vocab.CollectionPath("activities")
	actors     = vocab.CollectionPath("actors")
	objects    = vocab.CollectionPath("objects")
)

type Conf struct {
	SkipTLSVerify bool
	CachePath     string
	BaseURL       vocab.IRI
	UserAgent     string
	l             log.Logger
}

type fedbox struct {
	conf   Conf
	cred   *box.C2S
	pub    *vocab.Actor
	infoFn CtxLogFn
	errFn  CtxLogFn
}

type OptionFn func(*fedbox) error

func WithOAuth2(cred *box.C2S) OptionFn {
	return func(f *fedbox) error {
		f.cred = cred
		return nil
	}
}

func WithLogger(l log.Logger) OptionFn {
	return func(f *fedbox) error {
		f.conf.l = l
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
		f.conf.BaseURL = vocab.IRI(s)
		return nil
	}
}

func WithUserAgent(u string) OptionFn {
	return func(f *fedbox) error {
		f.conf.UserAgent = u
		return nil
	}
}

func SkipTLSCheck(skip bool) OptionFn {
	return func(f *fedbox) error {
		f.conf.SkipTLSVerify = skip
		return nil
	}
}

func NewClient(o ...OptionFn) (*fedbox, error) {
	ctx := context.Background()
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
	options = append(options,
		client.WithLogger(f.conf.l.WithContext(log.Ctx{"log": "client"})),
		client.SkipTLSValidation(f.conf.SkipTLSVerify),
	)

	var tr http.RoundTripper = &http.Transport{}
	if f.conf.CachePath != "" {
		tr = cache.Private(tr, cache.FS(f.conf.CachePath))
	}
	cl := f.Client(nil)
	service, err := cl.Actor(ctx, f.conf.BaseURL)
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
	if i.Contains(f.conf.BaseURL, false) {
		bu, be := f.conf.BaseURL.URL()
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
	it, err := f.Client(nil).CtxLoadIRI(ctx, i)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to load IRI: %s", i)
	}
	if vocab.IsNil(it) {
		return nil, errors.Newf("Unable to load IRI, nil item: %s", i)
	}
	typ := it.GetType()
	if !vocab.CollectionTypes.Match(it.GetType()) {
		return nil, errors.Errorf("Response item type is not a valid collection: %s", typ)
	}

	if !vocab.CollectionTypes.Match(typ) {
		return nil, errors.Errorf("Unable to convert item type %s to any of the collection types", typ)
	}
	return it.(vocab.CollectionInterface), nil
}

func (f fedbox) object(ctx context.Context, i vocab.IRI) (vocab.Item, error) {
	return f.Client(nil).CtxLoadIRI(ctx, f.normaliseIRI(i))
}

type LoadFn func(vocab.Item, ...filters.Check) vocab.IRI

func iri(i vocab.IRI, f ...filters.Check) vocab.IRI {
	return vocab.IRI(fmt.Sprintf("%s?%s", i, filters.ToValues(f...).Encode()))
}

func outbox(a vocab.Item, f ...filters.Check) vocab.IRI {
	return iri(vocab.Outbox.IRI(a), f...)
}

func followers(a vocab.Item, f ...filters.Check) vocab.IRI {
	return iri(vocab.Followers.IRI(a), f...)
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

func (f fedbox) Activities(ctx context.Context, filters ...filters.Check) (vocab.CollectionInterface, error) {
	return f.collection(ctx, iri(activities.IRI(f.Service()), filters...))
}

func (f fedbox) Actors(ctx context.Context, filters ...filters.Check) (vocab.CollectionInterface, error) {
	return f.collection(ctx, iri(actors.IRI(f.Service()), filters...))
}

func (f fedbox) Objects(ctx context.Context, filters ...filters.Check) (vocab.CollectionInterface, error) {
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

func (f *fedbox) Client(baseClient *http.Client) *client.C {
	if baseClient == nil {
		baseClient = &http.Client{Transport: cache.Private(http.DefaultTransport, cache.Mem(client.MByte))}
	}
	conf := f.conf
	return client.New(
		client.WithLogger(conf.l.WithContext(log.Ctx{"log": "client"})),
		client.WithHTTPClient(baseClient),
		client.SkipTLSValidation(conf.SkipTLSVerify),
		client.WithUserAgent(f.conf.UserAgent),
	)
}

func (r *repository) ToOutbox(ctx context.Context, cred box.C2S, a vocab.Item) (vocab.IRI, vocab.Item, error) {
	sendTo := vocab.IRI("")
	_ = vocab.OnActivity(a, func(a *vocab.Activity) error {
		sendTo = outbox(a.Actor)
		return nil
	})
	if err := validateIRIForRequest(sendTo); err != nil {
		return "", nil, errors.Annotatef(err, "Invalid Outbox IRI")
	}

	sendTo = r.fedbox.normaliseIRI(sendTo)

	// NOTE(marius): we avoid the cache transport for outgoing POST requests.
	cl := r.fedbox.Client(cred.Client(ctx))
	i, it, err := cl.CtxToCollection(ctx, a, sendTo)
	if err != nil {
		return i, a, err
	}

	_ = r.st.Save(it)
	if a, err = cl.LoadIRI(i); err == nil {
		_ = r.st.Save(a)
	}

	return i, it, nil
}

func (f *fedbox) Service() *vocab.Service {
	if f == nil || f.pub == nil {
		// TODO(marius): this should probably fail if there's no service actor for our FedBOX server
		return anonymousActor
	}
	return f.pub
}
