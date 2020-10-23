package app

import (
	"bytes"
	"fmt"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/mariusor/littr.go/internal/assets"
	"github.com/mariusor/littr.go/internal/config"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/writeas/go-nodeinfo"
	"net/http"
)

const (
	// Anonymous label
	Anonymous = "anonymous"
	// System label
	System = "system"
)

var (
	// AnonymousAccount
	AnonymousAccount = Account{Handle: Anonymous, Hash: AnonymousHash, Metadata: new(AccountMetadata)}
	// SystemAccount
	SystemAccount = Account{Handle: System, Hash: AnonymousHash, Metadata: new(AccountMetadata)}
)

var (
	listenHost string
	listenPort int64
	listenOn   string
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
func New(c *config.Configuration, host string, port int, ver string, m *chi.Mux) Application {
	app := Application{Version: ver, Mux: m}
	app.setUp(c, host, port)
	return app
}

func (a *Application) setUp(c *config.Configuration, host string, port int) error {
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

	r := a.Mux
	// Frontend
	r.With(front.Repository).Route("/", front.Routes(a.Conf))

	// .well-known
	cfg := NodeInfoConfig()
	ni := nodeinfo.NewService(cfg, NodeInfoResolverNew(front.storage.fedbox))
	// Web-Finger
	r.Route("/.well-known", func(r chi.Router) {
		r.Get("/webfinger", front.HandleWebFinger)
		//r.Get("/host-meta", h.HandleHostMeta)
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
