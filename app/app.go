package app

import (
	"context"
	"fmt"
	"github.com/adjust/redismq"
	"github.com/go-chi/chi/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jmoiron/sqlx"
	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

// Logger is the package default logger instance
var Logger log.FieldLogger

const defaultHost = "localhost"
const defaultPort = 3000

const (
	// Anonymous label
	Anonymous = "anonymous"
	// AnonymousHash is the sha hash for the anonymous account
	AnonymousHash = models.Hash("77b7b7215e8d78452dc40da9efbb65fdc918c757844387aa0f88143762495c6b")
)

var listenHost string
var listenPort int64
var listenOn string

// EnvType type alias
type EnvType string

// DEV environment
const DEV EnvType = "dev"

// PROD environment
const PROD EnvType = "prod"

// QA environment
const QA EnvType = "qa"

var validEnvTypes = []EnvType{
	DEV,
	PROD,
}

type config struct {
	Env                 EnvType
	DbBackendEnabled    bool
	ESBackendEnabled    bool
	RedisBackendEnabled bool
	SessionsEnabled     bool
	VotingEnabled       bool
	DownvotingEnabled   bool
	UserCreatingEnabled bool
}

// Stats holds data for keeping compatibility with Mastodon instances
type Stats struct {
	DomainCount int `json:"domain_count"`
	UserCount   int `json:"user_count"`
	StatusCount int `json:"status_count"`
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
	Version     string
	HostName    string
	BaseURL     string
	Port        int64
	Listen      string
	Db          *sqlx.DB
	Queues      map[string]*redismq.Queue
	Secure      bool
	SessionKeys [][]byte
	Config      config
}

// Instance is the default instance of our application
var Instance Application

func init() {
	if Logger == nil {
		Logger = log.StandardLogger()
	}
}

// New instantiates a new Application
func New() Application {
	app := Application{HostName: listenHost, Port: listenPort, Config: config{}}
	loadEnv(&app)
	return app
}

func validEnv(s EnvType) bool {
	for _, k := range validEnvTypes {
		if k == s {
			return true
		}
	}
	return false
}

// Name formats the name of the current Application
func (a Application) Name() string {
	parts := strings.Split(a.HostName, ".")
	return strings.Join(parts, " ")
}

func (a *Application) listen() string {
	if len(a.Listen) > 0 {
		return a.Listen
	}
	var port string
	if a.Port != 0 {
		port = fmt.Sprintf(":%d", a.Port)
	}
	return fmt.Sprintf("%s%s", a.HostName, port)
}

func loadEnv(l *Application) (bool, error) {
	if authKey := []byte(os.Getenv("SESS_AUTH_KEY")); authKey != nil {
		l.SessionKeys = append(l.SessionKeys, authKey)
		l.Config.SessionsEnabled = true
	}
	if encKey := []byte(os.Getenv("SESS_ENC_KEY")); encKey != nil {
		l.SessionKeys = append(l.SessionKeys, encKey)
	}

	listenHost = os.Getenv("HOSTNAME")
	listenPort, _ = strconv.ParseInt(os.Getenv("PORT"), 10, 64)
	listenOn = os.Getenv("LISTEN")

	l.Version = os.Getenv("VERSION")
	env := EnvType(os.Getenv("ENV"))
	if !validEnv(env) {
		env = DEV
	}
	Instance.Config.Env = env
	if listenPort == 0 {
		listenPort = defaultPort
	}
	if listenHost == "" {
		listenHost = defaultHost
	}
	l.HostName = listenHost
	if l.Secure {
		l.BaseURL = fmt.Sprintf("https://%s", listenHost)
	} else {
		l.BaseURL = fmt.Sprintf("http://%s", listenHost)
	}
	l.Secure = os.Getenv("HTTPS") != ""

	l.Port = listenPort
	l.Listen = listenOn

	dbHost := os.Getenv("DB_HOST")
	if len(dbHost) > 0 {
		dbPw := os.Getenv("DB_PASSWORD")
		dbName := os.Getenv("DB_NAME")
		dbUser := os.Getenv("DB_USER")

		connStr := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable", dbHost, dbUser, dbPw, dbName)

		db, err := sqlx.Open("postgres", connStr)
		if err != nil {
			new := errors.NewErr("failed to connect to the database")
			log.WithFields(log.Fields{
				"dbName":   dbName,
				"dbUser":   dbUser,
				"previous": err.Error(),
				"trace":    new.StackTrace(),
			}).Error(new)
			return false, err
		}
		l.Db = db
		l.Config.DbBackendEnabled = true
	} else {
		return false, errors.New("no database connection configuration, unable to continue")
	}

	redisHost := os.Getenv("REDIS_HOST")
	if len(redisHost) > 0 {
		redisPort := os.Getenv("REDIS_PORT")
		redisPw := os.Getenv("REDIS_PASSWORD")
		redisDb := 0
		name := "queue"
		red := redismq.CreateQueue(redisHost, redisPort, redisPw, int64(redisDb), name)
		if red == nil {
			new := errors.NewErr("failed to connect to redis")

			log.WithFields(log.Fields{
				"redisHost": redisHost,
				"redisPort": redisPort,
				"redisDb":   redisDb,
				"name":      name,
				"trace":     new.StackTrace(),
			}).Error(new)
			return false, &new
		}
		l.Queues[name] = red
		l.Config.RedisBackendEnabled = true
	}

	l.Config.VotingEnabled = os.Getenv("DISABLE_VOTING") == ""
	l.Config.DownvotingEnabled = os.Getenv("DISABLE_DOWNVOTING") == ""
	l.Config.SessionsEnabled = os.Getenv("DISABLE_SESSIONS") == ""

	return true, nil
}

// Run is the wrapper for starting the web-server and handling signals
func (a *Application) Run(m http.Handler, wait time.Duration) {
	Logger.WithFields(log.Fields{}).Infof("Starting debug level %q", log.GetLevel().String())
	Logger.WithFields(log.Fields{}).Infof("Listening on %s", a.listen())

	srv := &http.Server{
		Addr: a.listen(),
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      m,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			Logger.WithFields(log.Fields{}).Error(err)
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
				Logger.Info("SIGHUP received, reloading configuration")
				loadEnv(a)
			// kill -SIGINT XXXX or Ctrl+c
			case syscall.SIGINT:
				Logger.Info("SIGINT received, stopping")
				exitChan <- 0
			// kill -SIGTERM XXXX
			case syscall.SIGTERM:
				Logger.Info("SIGITERM received, force stopping")
				exitChan <- 0
			// kill -SIGQUIT XXXX
			case syscall.SIGQUIT:
				Logger.Info("SIGQUIT received, force stopping with core-dump")
				exitChan <- 0
			default:
				Logger.Info("Unknown signal %d.", s)
			}
		}
	}()
	code := <-exitChan

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	log.RegisterExitHandler(cancel)
	defer cancel()

	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	Logger.WithFields(log.Fields{}).Infof("Shutting down")
	os.Exit(code)
}

// ShowHeaders is a middleware for logging headers for a request
func ShowHeaders(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		for name, val := range r.Header {
			Logger.Infof("%s: %s", name, val)
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// StripCookies is a middleware for removing Header and SetCookie headers
func StripCookies(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Del("Set-Cookie")
		r.Header.Del("Cookie")
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func ReqLogger(next http.Handler) http.Handler {
	return middleware.DefaultLogger(next)
}

type errorHandler func(http.ResponseWriter, *http.Request, int, ...error)
type handler func(http.Handler) http.Handler

func NeedsDBBackend(fn errorHandler) handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			if !Instance.Config.DbBackendEnabled {
				fn(w, r, http.StatusInternalServerError, errors.New("db backend is disabled, can not continue"))
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}
