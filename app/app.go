package app

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mariusor/go-littr/internal/config"
	"github.com/mariusor/go-littr/internal/log"
	"github.com/writeas/go-nodeinfo"
)

const (
	// Deleted label
	Deleted = "deleted"
	// Anonymous label
	Anonymous = "anonymous"
	// System label
	System = "system"
)

var (
	// DeletedAccount is a default static value for a deleted account
	DeletedAccount = Account{Handle: Anonymous, Hash: AnonymousHash, Metadata: new(AccountMetadata), Pub: &vocab.Tombstone{}}
	// AnonymousAccount is a default static value for the anonymous account
	AnonymousAccount = Account{Handle: Anonymous, Hash: AnonymousHash, Metadata: new(AccountMetadata)}
	// SystemAccount is a default static value for the system account
	SystemAccount = Account{Handle: System, Hash: SystemHash, Metadata: new(AccountMetadata)}
	// DeletedItem is a default static value for a deleted item
	DeletedItem = Item{Title: Deleted, Hash: AnonymousHash, Metadata: new(ItemMetadata), Pub: &vocab.Tombstone{}}

	// cut off date for disallowing interactions with items
	oneYearishAgo = time.Now().Add(-12 * 30 * 24 * time.Hour).UTC()
)

// Application is the global state of our application
type Application struct {
	Version string
	BaseURL string
	Conf    *config.Configuration
	ModTags TagCollection
	Logger  log.Logger
	front   *handler
	Mux     *chi.Mux
}

// Instance is the default instance of our application
var Instance *Application

// New instantiates a new Application
func New(c *config.Configuration, l log.Logger, host string, port int, ver string) (*Application, error) {
	Instance = &Application{Version: ver}
	if err := Instance.init(c, l, host, port); err != nil {
		return nil, err
	}
	return Instance, nil
}

func (a *Application) Reload() error {
	a.Conf = config.Load(a.Conf.Env, a.Conf.TimeOut)
	a.front.storage.cache.remove()
	return nil
}

func (a *Application) init(c *config.Configuration, l log.Logger, host string, port int) error {
	a.Conf = c
	a.Logger = l
	if len(c.HostName) == 0 {
		c.HostName = host
	}
	if c.Secure {
		a.BaseURL = fmt.Sprintf("https://%s", c.HostName)
	} else {
		a.BaseURL = fmt.Sprintf("http://%s", c.HostName)
	}
	if c.AdminContact == "" {
		c.AdminContact = author
	}
	if host != "" {
		c.HostName = host
	}
	if port != config.DefaultListenPort {
		c.ListenPort = port
	}
	if c.APIURL == "" {
		c.APIURL = fmt.Sprintf("%s/api", a.BaseURL)
	}
	if err := a.Front(); err != nil {
		return err
	}
	a.Routes()
	return nil
}

func (a *Application) Front() error {
	conf := appConfig{
		Configuration: *a.Conf,
		BaseURL:       a.BaseURL,
		Logger:        a.Logger.New(log.Ctx{"package": "frontend"}),
	}
	a.front = new(handler)
	if err := a.front.init(conf); err != nil {
		return err
	}
	a.ModTags = a.front.storage.modTags
	return nil
}

func (a *Application) Routes() {
	// Routes
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	if !a.Conf.Env.IsProd() {
		r.Use(middleware.Recoverer)
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	a.Mux = r
	// Frontend
	r.With(a.front.Repository).Route("/", a.front.Routes(a.Conf))

	// .well-known
	cfg := NodeInfoConfig()
	ni := nodeinfo.NewService(cfg, NodeInfoResolverNew(a.front.storage))
	// Web-Finger
	r.Route("/.well-known", func(r chi.Router) {
		r.Get("/webfinger", a.front.HandleWebFinger)
		r.Get("/host-meta", a.front.HandleHostMeta)
		r.Get("/nodeinfo", ni.NodeInfoDiscover)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			errors.HandleError(errors.NotFoundf("%s", r.RequestURI)).ServeHTTP(w, r)
		})
	})
	r.Get("/nodeinfo", ni.NodeInfo)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		a.front.v.HandleErrors(w, r, errors.NotFoundf("%s", r.RequestURI))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		a.front.v.HandleErrors(w, r, errors.MethodNotAllowedf("%s not allowed", r.Method))
	})
}

type Cacheable interface {
	GetAge() int
}

func ReqLogger(f middleware.LogFormatter) Handler {
	return middleware.RequestLogger(f)
}

type Handler func(http.Handler) http.Handler
type ErrorHandler func(http.ResponseWriter, *http.Request, ...error)
type ErrorHandlerFn func(eh ErrorHandler) Handler

func Contains[T Renderable](sl []T, it T) bool {
	if !it.IsValid() || it.AP() == nil {
		return false
	}
	itIRI := it.AP().GetLink()
	for _, vv := range sl {
		if ap := vv.AP(); ap != nil && ap.GetLink() == itIRI {
			return true
		}
	}
	return false
}
