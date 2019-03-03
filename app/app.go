package app

import (
	"bytes"
	"context"
	"fmt"
	"github.com/go-chi/chi/middleware"
	"github.com/joho/godotenv"
	"github.com/mariusor/littr.go/internal/errors"
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
	// AnonymousHash is the sha hash for the anonymous account
	AnonymousHash = Hash("eacff9ddf379bd9fc8274c5a9f4cae08")
)

var AnonymousAccount = Account{Handle: Anonymous, Hash: AnonymousHash}

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

type Config struct {
	Env                 EnvType
	DB                  backendConfig
	ES                  backendConfig
	Redis               backendConfig
	SessionsEnabled     bool
	VotingEnabled       bool
	DownvotingEnabled   bool
	UserCreatingEnabled bool
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
	Config   Config
	Logger   log.Logger
	SeedVal  int64
}

type Collection interface{}

// Instance is the default instance of our application
var Instance Application

// New instantiates a new Application
func New(host string, port int, env EnvType, ver string) Application {
	app := Application{HostName: host, Port: port, Version: ver, Config: Config{ Env: env}}
	loadEnv(&app)
	return app
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
	return  strings.Contains(string(e), string(PROD))
}
func (e EnvType) IsQA() bool {
	return  strings.Contains(string(e), string(QA))
}
func (e EnvType) IsTest() bool {
	return  strings.Contains(string(e), string(TEST))
}

// Name formats the name of the current Application
func (a Application) Name() string {
	parts := strings.Split(a.HostName, ".")
	return strings.Join(parts, " ")
}

// Name formats the name of the current Application
func (a Application) NodeInfo() Info {
	inf := Info{
		Title:   a.Name(),
		Summary: "Link aggregator inspired by reddit and hacker news using ActivityPub federation.",
		Email:   "system@littr.me",
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
	var err error

	if !validEnv(l.Config.Env) {
		l.Config.Env = EnvType(os.Getenv("ENV"))
	}
	if !validEnv(l.Config.Env) {
		l.Config.Env = DEV
	}
	configs := []string{
		".env",
		fmt.Sprintf(".env.%s", l.Config.Env),
	}
	if l.Config.Env == PROD {
		l.Logger = log.Prod()
	} else {
		l.Logger = log.Dev()
	}

	for _, f := range configs {
		if err := godotenv.Overload(f); err != nil {
			l.Logger.Warnf("%s", err)
		}
	}

	if l.HostName == "" {
		l.HostName = os.Getenv("HOSTNAME")
		if l.HostName == "" {
			l.HostName = DefaultHost
		}
	}
	if l.listen = os.Getenv("LISTEN"); l.listen == "" {
		l.listen = fmt.Sprintf("%s:%d", l.HostName, l.Port)
	}
	if l.SeedVal, err = strconv.ParseInt(os.Getenv("SEED"), 10, 64); err != nil {
		l.SeedVal = 666
	}
	if l.Secure, err = strconv.ParseBool(os.Getenv("HTTPS")); err != nil {
		l.Secure = false
	}
	if l.Secure {
		l.BaseURL = fmt.Sprintf("https://%s", l.HostName)
	} else {
		l.BaseURL = fmt.Sprintf("http://%s", l.HostName)
	}

	l.Config.DB.Host = os.Getenv("DB_HOST")
	l.Config.DB.Pw = os.Getenv("DB_PASSWORD")
	l.Config.DB.Name = os.Getenv("DB_NAME")
	l.Config.DB.Port = os.Getenv("DB_Port")
	l.Config.DB.User = os.Getenv("DB_USER")

	l.Config.Redis.Host = os.Getenv("REDIS_HOST")
	l.Config.Redis.Port = os.Getenv("REDIS_PORT")
	l.Config.Redis.Pw = os.Getenv("REDIS_PASSWORD")

	l.Config.VotingEnabled = os.Getenv("DISABLE_VOTING") == ""
	l.Config.DownvotingEnabled = os.Getenv("DISABLE_DOWNVOTING") == ""
	l.Config.SessionsEnabled = os.Getenv("DISABLE_SESSIONS") == ""

	return true, nil
}

// Run is the wrapper for starting the web-server and handling signals
func (a *Application) Run(m http.Handler, wait time.Duration) {
	a.Logger.WithContext(log.Ctx{
		"listen": a.Listen(),
		"host":   a.HostName,
		"env":    a.Config.Env,
	}).Info("Started")
	srv := &http.Server{
		Addr: a.Listen(),
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      m,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			a.Logger.Error(err.Error())
			os.Exit(1)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT,
		syscall.SIGTERM, syscall.SIGQUIT)

	exitChan := make(chan int)
	go func() {
		for {
			s := <-sigChan
			switch s {
			case syscall.SIGHUP:
				a.Logger.Info("SIGHUP received, reloading configuration")
				loadEnv(a)
			// kill -SIGINT XXXX or Ctrl+c
			case syscall.SIGINT:
				a.Logger.Info("SIGINT received, stopping")
				exitChan <- 0
			// kill -SIGTERM XXXX
			case syscall.SIGTERM:
				a.Logger.Info("SIGITERM received, force stopping")
				exitChan <- 0
			// kill -SIGQUIT XXXX
			case syscall.SIGQUIT:
				a.Logger.Info("SIGQUIT received, force stopping with core-dump")
				exitChan <- 0
			default:
				a.Logger.WithContext(log.Ctx{"signal": s}).Info("Unknown signal")
			}
		}
	}()
	code := <-exitChan

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	//log.RegisterExitHandler(cancel)
	defer cancel()

	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	a.Logger.Info("Shutting down")
	os.Exit(code)
}

// ShowHeaders is a middleware for logging headers for a request
func ShowHeaders(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		for name, val := range r.Header {
			Logger.WithContext(log.Ctx{name: val}).Info("")
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// StripCookies is a middleware for removing Header and SetCookie headers
func StripCookies(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		w.Header().Del("Set-Cookie")
		w.Header().Set("Cache-Control", "no-cache")
	}
	return http.HandlerFunc(fn)
}

func ReqLogger(next http.Handler) http.Handler {
	return middleware.DefaultLogger(next)
}

type Handler func(http.Handler) http.Handler
type ErrorHandler func(http.ResponseWriter, *http.Request, ...error)
type ErrorHandlerFn func(eh ErrorHandler) Handler

func NeedsDBBackend(eh ErrorHandler) Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			if !Instance.Config.DB.Enabled {
				eh(w, r, errors.NotValidf("db backend is disabled, can not continue"))
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}
