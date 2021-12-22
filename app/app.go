package app

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mariusor/go-littr/internal/assets"
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
	DeletedAccount = Account{Handle: Anonymous, Hash: AnonymousHash, Metadata: new(AccountMetadata), pub: &pub.Tombstone{}}
	// AnonymousAccount is a default static value for the anonymous account
	AnonymousAccount = Account{Handle: Anonymous, Hash: AnonymousHash, Metadata: new(AccountMetadata)}
	// SystemAccount is a default static value for the system account
	SystemAccount = Account{Handle: System, Hash: SystemHash, Metadata: new(AccountMetadata)}
	// DeletedItem is a default static value for a deleted item
	DeletedItem = Item{Title: Deleted, Hash: AnonymousHash, Metadata: new(ItemMetadata), pub: &pub.Tombstone{}}

	// cut off date for disallowing interactions with items
	oneYearishAgo = time.Now().Add(-12 * 30 * 24 * time.Hour).UTC()
)

// Stats holds data for keeping compatibility with Mastodon instances
type Stats struct {
	DomainCount int  `json:"domain_count"`
	UserCount   uint `json:"user_count"`
	StatusCount uint `json:"status_count"`
}

// Desc holds data for keeping compatibility with Mastodon instances
type Desc struct {
	Description string   `json:"description"`
	Email       string   `json:"email"`
	Stats       Stats    `json:"stats"`
	Thumbnail   string   `json:"thumbnail,omitempty"`
	Title       string   `json:"title"`
	Lang        []string `json:"languages"`
	URI         string   `json:"uri"`
	Urls        []string `json:"urls,omitempty"`
	Version     string   `json:"version"`
}

// Application is the global state of our application
type Application struct {
	Version string
	BaseURL string
	Conf    *config.Configuration
	Logger  log.Logger
	front   *handler
	Mux     *chi.Mux
}

type Collection interface{}

// Instance is the default instance of our application
var Instance Application

// New instantiates a new Application
func New(c *config.Configuration, host string, port int, ver string) Application {
	app := Application{Version: ver}
	app.init(c, host, port)
	return app
}

func (a *Application) Reload() error {
	a.Conf = config.Load(a.Conf.Env, a.Conf.TimeOut)
	a.front.storage.cache.remove()
	return nil
}

func (a *Application) init(c *config.Configuration, host string, port int) error {
	a.Conf = c
	a.Logger = log.Dev(c.LogLevel)
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
	Instance = *a
	a.Front()
	return nil
}

func (a *Application) Front() error {
	conf := appConfig{
		Configuration: *a.Conf,
		BaseURL:       a.BaseURL,
		Logger:        a.Logger.New(log.Ctx{"package": "frontend"}),
	}
	front, err := Init(conf)
	if err != nil {
		a.Logger.Error(err.Error())
		return err
	}
	a.front = front

	// Routes
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	if !a.Conf.Env.IsProd() {
		r.Use(middleware.Recoverer)
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	a.Mux = r
	// Frontend
	r.With(front.Repository).Route("/", front.Routes(a.Conf))

	// .well-known
	cfg := NodeInfoConfig()
	ni := nodeinfo.NewService(cfg, NodeInfoResolverNew(front.storage, front.conf.Env))
	// Web-Finger
	r.Route("/.well-known", func(r chi.Router) {
		r.Get("/webfinger", front.HandleWebFinger)
		r.Get("/host-meta", front.HandleHostMeta)
		r.Get("/nodeinfo", ni.NodeInfoDiscover)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			errors.HandleError(errors.NotFoundf("%s", r.RequestURI)).ServeHTTP(w, r)
		})
	})
	r.Get("/nodeinfo", ni.NodeInfo)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		front.v.HandleErrors(w, r, errors.NotFoundf("%s", r.RequestURI))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		front.v.HandleErrors(w, r, errors.MethodNotAllowedf("%s not allowed", r.Method))
	})
	return nil
}

type Cacheable interface {
	GetAge() int
}

func (a Application) NodeInfo() WebInfo {
	// Name formats the name of the current Application
	inf := WebInfo{
		Title:   a.Conf.Name,
		Summary: "Link aggregator inspired by reddit and hacker news using ActivityPub federation.",
		Email:   a.Conf.AdminContact,
		URI:     a.BaseURL,
		Version: a.Version,
	}

	if desc, err := assets.GetFullFile("./README.md"); err == nil {
		inf.Description = string(bytes.Trim(desc, "\x00"))
	}
	return inf
}

func ReqLogger(f middleware.LogFormatter) Handler {
	return middleware.RequestLogger(f)
}

type Handler func(http.Handler) http.Handler
type ErrorHandler func(http.ResponseWriter, *http.Request, ...error)
type ErrorHandlerFn func(eh ErrorHandler) Handler
