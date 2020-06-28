package app

import (
	"bytes"
	"context"
	"fmt"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/mariusor/littr.go/internal/config"
	"github.com/writeas/go-nodeinfo"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mariusor/littr.go/internal/log"
)

// Logger is the package default logger instance
var Logger log.Logger

const (
	// Anonymous label
	Anonymous = "anonymous"
	System    = "system"
)

// AnonymousHash is the sha hash for the anonymous account
var AnonymousHash = Hash{}
var AnonymousAccount = Account{Handle: Anonymous, Hash: AnonymousHash, Metadata: &AccountMetadata{}}
var SystemAccount = Account{Handle: System, Hash: AnonymousHash, Metadata: &AccountMetadata{}}

var listenHost string
var listenPort int64
var listenOn string

const DefaultHost = "localhost"

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
}

type Collection interface{}

// Instance is the default instance of our application
var Instance Application

// New instantiates a new Application
func New(host string, port int, env config.EnvType, ver string) Application {
	c, err := config.Load(env)
	if err != nil {
		c = &config.Default
	}
	app := Application{Version: ver, Conf: c}
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
	if port != 0 {
		c.Port = port
	}
	if c.APIURL == "" {
		c.APIURL = fmt.Sprintf("%s/api", a.BaseURL)
	}
	return nil
}

func (a Application) Listen() string {
	return fmt.Sprintf("%s:%d", a.Conf.HostName, a.Conf.Port)
}

func (a *Application) Front(r chi.Router) {
	conf := appConfig{
		Configuration: *a.Conf,
		BaseURL:  a.BaseURL,
		Logger:   a.Logger.New(log.Ctx{"package": "frontend"}),
	}
	front, err := Init(conf)
	if err != nil {
		a.Logger.Error(err.Error())
	}
	a.front = front

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

	if f, err := os.Open("./README.md"); err == nil {
		st, _ := f.Stat()
		rme := make([]byte, st.Size())
		io.ReadFull(f, rme)
		inf.Description = string(bytes.Trim(rme, "\x00"))
	}
	return inf
}

type exit struct {
	// signal is a channel which is waiting on user/os signals
	signal chan os.Signal
	// status is a channel on which we return exit codes for application
	status chan int
	// handlers is the mapping of signals to functions to execute
	h signalHandlers
}

type signalHandlers map[os.Signal]func(*exit, os.Signal)

// RegisterSignalHandlers sets up the signal handlers we want to use
func RegisterSignalHandlers(handlers signalHandlers) *exit {
	x := &exit{
		signal: make(chan os.Signal, 1),
		status: make(chan int, 1),
		h:      handlers,
	}
	signals := make([]os.Signal, 0)
	for sig := range handlers {
		signals = append(signals, sig)
	}
	signal.Notify(x.signal, signals...)
	return x
}

// handle reads signals received from the os and executes the handlers it has registered
func (ex *exit) wait() chan int {
	go func(ex *exit) {
		for {
			select {
			case s := <-ex.signal:
				ex.h[s](ex, s)
			}
		}
	}(ex)
	return ex.status
}

// SetupHttpServer creates a new http server and returns the start and stop functions for it
func SetupHttpServer(listen string, m http.Handler, wait time.Duration, ctx context.Context) (func() error, func() error) {
	srv := &http.Server{
		Addr:         listen,
		WriteTimeout: wait,
		ReadTimeout:  wait,
		IdleTimeout:  time.Second * 60,
		Handler:      m,
	}

	shutdown := func() error {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != http.ErrServerClosed {
				return err
			}
		}
		err := srv.Shutdown(ctx)
		if err != nil {
			return err
		}
		return nil
	}

	// Run our server in a goroutine so that it doesn't block.
	return srv.ListenAndServe, shutdown
}

// Run is the wrapper for starting the web-server and handling signals
func (a *Application) Run(m http.Handler, wait time.Duration) {
	a.Logger.WithContext(log.Ctx{
		"listen": a.Listen(),
		"host":   a.Conf.HostName,
		"env":    a.Conf.Env,
	}).Info("Started")

	srvStart, srvShutdown := SetupHttpServer(a.Listen(), m, wait, context.Background())
	defer srvShutdown()

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srvStart(); err != nil {
			a.Logger.Errorf("Error: %s", err)
			os.Exit(1)
		}
	}()

	// Set up the signal channel to tell us if the user/os requires us to stop
	sigHandlerFns := signalHandlers{
		syscall.SIGHUP: func(x *exit, s os.Signal) {
			a.Logger.Info("SIGHUP received, reloading configuration")
			if c, err := config.Load(a.Conf.Env); err == nil {
				a.Conf = c
			}
		},
		syscall.SIGUSR1: func(x *exit, s os.Signal) {
			a.Logger.Info("SIGUSR1 received, switching to maintenance mode")
			a.Conf.MaintenanceMode = !a.Conf.MaintenanceMode
		},
		syscall.SIGTERM: func(x *exit, s os.Signal) {
			// kill -SIGTERM XXXX
			a.Logger.Info("SIGTERM received, stopping")
			x.status <- 0
		},
		syscall.SIGINT: func(x *exit, s os.Signal) {
			// kill -SIGINT XXXX or Ctrl+c
			a.Logger.Info("SIGINT received, stopping")
			x.status <- 0
		},
		syscall.SIGQUIT: func(x *exit, s os.Signal) {
			a.Logger.Error("SIGQUIT received, force stopping")
			x.status <- -1
		},
	}

	// Wait for OS signals asynchronously
	code := <-RegisterSignalHandlers(sigHandlerFns).wait()
	if code == 0 {
		a.Logger.Info("Shutting down")
	}
	os.Exit(code)
}

func ReqLogger(f middleware.LogFormatter) Handler {
	return middleware.RequestLogger(f)
}

type Handler func(http.Handler) http.Handler
type ErrorHandler func(http.ResponseWriter, *http.Request, ...error)
type ErrorHandlerFn func(eh ErrorHandler) Handler
