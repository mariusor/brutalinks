package app

import (
	"bytes"
	"context"
	"fmt"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/joho/godotenv"
	"github.com/writeas/go-nodeinfo"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
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

// EnvType type alias
type EnvType string

// DEV environment
const DEV EnvType = "dev"

// PROD environment
const PROD EnvType = "prod"

// QA environment
const QA EnvType = "qa"

// testing environment
const TEST EnvType = "test"

var validEnvTypes = []EnvType{
	DEV,
	PROD,
	QA,
	TEST,
}

type backendConfig struct {
	Enabled bool
	Host    string
	Port    string
	User    string
	Pw      string
	Name    string
}

type Configuration struct {
	Name                       string
	Env                        EnvType
	LogLevel                   log.Level
	DB                         backendConfig
	AdminContact               string
	AnonymousCommentingEnabled bool
	SessionsEnabled            bool
	VotingEnabled              bool
	DownvotingEnabled          bool
	UserCreatingEnabled        bool
	UserFollowingEnabled       bool
	ModerationEnabled          bool
	MaintenanceMode            bool
}

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
	Version  string
	HostName string
	APIURL   string
	BaseURL  string
	Port     int
	listen   string
	Secure   bool
	Config   Configuration
	Logger   log.Logger
	front    *handler
}

type Collection interface{}

// Instance is the default instance of our application
var Instance Application

// New instantiates a new Application
func New(host string, port int, env EnvType, ver string) Application {
	app := Application{HostName: host, Port: port, Version: ver, Config: Configuration{Env: env}}
	loadEnv(&app)
	return app
}

func (a *Application) Front(r chi.Router) {
	conf := appConfig{
		Name:     a.Config.Name,
		Env:      a.Config.Env,
		Logger:   a.Logger.New(log.Ctx{"package": "frontend"}),
		Secure:   a.Secure,
		BaseURL:  a.BaseURL,
		APIURL:   a.APIURL,
		HostName: a.HostName,
	}
	front, err := Init(conf)
	if err != nil {
		a.Logger.Error(err.Error())
		//return
	}
	a.front = front

	// Frontend
	r.With(front.Repository).Route("/", front.Routes())

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

func validEnv(env EnvType) bool {
	if len(env) == 0 {
		return false
	}
	s := strings.ToLower(string(env))
	for _, k := range validEnvTypes {
		if strings.Contains(s, string(k)) {
			return true
		}
	}
	return false
}

func (e EnvType) IsProd() bool {
	return strings.Contains(string(e), string(PROD))
}
func (e EnvType) IsQA() bool {
	return strings.Contains(string(e), string(QA))
}
func (e EnvType) IsTest() bool {
	return strings.Contains(string(e), string(TEST))
}
func (e EnvType) IsDev() bool {
	return strings.Contains(string(e), string(DEV))
}

func (a Application) NodeInfo() WebInfo {
	// Name formats the name of the current Application
	inf := WebInfo{
		Title:   a.Config.Name,
		Summary: "Link aggregator inspired by reddit and hacker news using ActivityPub federation.",
		Email:   a.Config.AdminContact,
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

func (a *Application) Listen() string {
	if len(a.listen) > 0 {
		return a.listen
	}
	var port string
	if a.Port != 0 {
		port = fmt.Sprintf(":%d", a.Port)
	}
	return fmt.Sprintf("%s%s", a.HostName, port)
}

func loadEnv(l *Application) (bool, error) {
	if !validEnv(l.Config.Env) {
		env := os.Getenv("ENV")
		l.Config.Env = EnvType(strings.ToLower(env))
	}
	if !validEnv(l.Config.Env) {
		l.Config.Env = DEV
	}
	configs := []string{
		".env",
		fmt.Sprintf(".env.%s", l.Config.Env),
	}

	lvl := os.Getenv("LOG_LEVEL")
	switch strings.ToLower(lvl) {
	case "trace":
		l.Config.LogLevel = log.TraceLevel
	case "debug":
		l.Config.LogLevel = log.DebugLevel
	case "warn":
		l.Config.LogLevel = log.WarnLevel
	case "error":
		l.Config.LogLevel = log.ErrorLevel
	case "info":
		fallthrough
	default:
		l.Config.LogLevel = log.InfoLevel
	}
	l.Logger = log.Dev(l.Config.LogLevel)

	for _, f := range configs {
		if err := godotenv.Overload(f); err != nil {
			l.Logger.Warnf("%s", err)
		}
	}

	l.HostName = os.Getenv("HOSTNAME")
	if l.HostName == "" {
		l.HostName = DefaultHost
	}
	l.Config.Name = os.Getenv("NAME")
	if l.Config.Name == "" {
		l.Config.Name = l.HostName
	}
	if l.listen = os.Getenv("LISTEN"); l.listen == "" {
		l.listen = fmt.Sprintf("%s:%d", l.HostName, l.Port)
	}
	l.Secure, _ = strconv.ParseBool(os.Getenv("HTTPS"))
	if l.Secure {
		l.BaseURL = fmt.Sprintf("https://%s", l.HostName)
	} else {
		l.BaseURL = fmt.Sprintf("http://%s", l.HostName)
	}

	l.Config.DB.Host = os.Getenv("DB_HOST")
	l.Config.DB.Pw = os.Getenv("DB_PASSWORD")
	l.Config.DB.Name = os.Getenv("DB_NAME")
	l.Config.DB.Port = os.Getenv("DB_PORT")
	l.Config.DB.User = os.Getenv("DB_USER")

	votingDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_VOTING"))
	l.Config.VotingEnabled = !votingDisabled
	if l.Config.VotingEnabled {
		downvotingDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_DOWNVOTING"))
		l.Config.DownvotingEnabled = !downvotingDisabled
	}
	sessionsDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_SESSIONS"))
	l.Config.SessionsEnabled = !sessionsDisabled
	userCreationDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_USER_CREATION"))
	l.Config.UserCreatingEnabled = !userCreationDisabled
	// TODO(marius): this stopped working - as the anonymous user doesn't have a valid Outbox.
	anonymousCommentingDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_ANONYMOUS_COMMENTING"))
	l.Config.AnonymousCommentingEnabled = !anonymousCommentingDisabled
	userFollowingDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_USER_FOLLOWING"))
	l.Config.UserFollowingEnabled = !userFollowingDisabled
	moderationDisabled, _ := strconv.ParseBool(os.Getenv("DISABLE_MODERATION"))
	l.Config.ModerationEnabled = !moderationDisabled
	l.Config.AdminContact = os.Getenv("ADMIN_CONTACT")

	if l.Config.AdminContact == "" {
		l.Config.AdminContact = author
	}

	if l.APIURL = os.Getenv("API_URL"); l.APIURL == "" {
		l.APIURL = fmt.Sprintf("%s/api", l.BaseURL)
	}
	return true, nil
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
		"host":   a.HostName,
		"env":    a.Config.Env,
	}).Info("Started")

	srvStart, srvShutdown := SetupHttpServer(a.Listen(), m, wait, context.Background())
	defer srvShutdown()

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srvStart(); err != nil {
			a.Logger.Error(err.Error())
			os.Exit(1)
		}
	}()

	// Set up the signal channel to tell us if the user/os requires us to stop
	sigHandlerFns := signalHandlers{
		syscall.SIGHUP: func(x *exit, s os.Signal) {
			a.Logger.Info("SIGHUP received, reloading configuration")
			loadEnv(a)
		},
		syscall.SIGUSR1: func(x *exit, s os.Signal) {
			a.Logger.Info("SIGUSR1 received, switching to maintenance mode")
			a.Config.MaintenanceMode = !a.Config.MaintenanceMode
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
