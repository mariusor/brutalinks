package main

import (
	"database/sql"
	"encoding/gob"
	"flag"
	"fmt"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/gorilla/sessions"
	"github.com/juju/errors"
	_ "github.com/lib/pq"
	"github.com/mariusor/littr.go/api"
	"github.com/mariusor/littr.go/app"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"github.com/mariusor/littr.go/models"
)

const defaultHost = "localhost"
const defaultPort = 3000

var listenHost string
var listenPort int64
var listenOn string

var littr app.Littr

func loadEnv(l *app.Littr) (bool, error) {
	l.SessionKeys[0] = []byte(os.Getenv("SESS_AUTH_KEY"))
	l.SessionKeys[1] = []byte(os.Getenv("SESS_ENC_KEY"))

	listenHost = os.Getenv("HOSTNAME")
	listenPort, _ = strconv.ParseInt(os.Getenv("PORT"), 10, 64)
	listenOn = os.Getenv("LISTEN")

	env := app.EnvType(os.Getenv("ENV"))
	if !app.ValidEnv(env) {
		env = app.DEV
	}
	littr.Env = env
	if listenPort == 0 {
		listenPort = defaultPort
	}
	if listenHost == "" {
		listenHost = defaultHost
	}
	l.HostName = listenHost
	l.Port = listenPort
	l.Listen = listenOn

	return true, nil
}

func init() {
	littr = app.Littr{HostName: listenHost, Port: listenPort}

	loadEnv(&littr)

	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stdout)

	if littr.Env == app.PROD {
		log.SetLevel(log.WarnLevel)
	} else {
		log.SetLevel(log.DebugLevel)
	}
	gob.Register(models.Account{})
	gob.Register(app.Flash{})

	s := sessions.NewCookieStore(littr.SessionKeys[0], littr.SessionKeys[1])
	//s.Options.Domain = littr.HostName
	s.Options.Path = "/"
	app.SessionStore = s

	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Error(errors.NewErrWithCause(err, "failed to connect to the database"))
	}

	app.Db = db
	models.Db = db
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit URL parameters.")
	}

	fs := http.StripPrefix(path, http.FileServer(root))

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	}))
}

func main() {
	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	logger := log.New()
	middleware.DefaultLogger = middleware.RequestLogger(&middleware.DefaultLogFormatter{Logger: logger})
	// Routes
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	if littr.Env == app.PROD {
		r.Use(middleware.Recoverer)
	}

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		app.HandleError(w, r, http.StatusNotFound, errors.Errorf("%s not found", r.RequestURI))
	})

	workDir, _ := os.Getwd()
	filesDir := filepath.Join(workDir, "assets")
	FileServer(r, "/assets", http.Dir(filesDir))

	r.With(api.Loader).Route("/", func(r chi.Router) {
		r.Use(app.LoadSessionData)

		r.Get("/", app.HandleIndex)

		r.Get("/submit", app.ShowSubmit)
		r.Post("/submit", app.HandleSubmit)

		r.Get("/register", app.ShowRegister)
		r.Post("/register", app.HandleRegister)

		r.Get("/~{handle}", app.HandleUser)

		//r.With(Item).Get("/~{handle}/{hash}", app.ShowItem)
		r.Get("/~{handle}/{hash}", app.ShowItem)
		r.Post("/~{handle}/{hash}", app.HandleSubmit)
		r.Get("/~{handle}/{hash}/{direction}", app.HandleVoting)

		//r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/", app.HandleDate)
		r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", app.ShowItem)
		r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}/{direction}", app.HandleVoting)
		r.Post("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", app.HandleSubmit)

		r.Get("/parent/{hash}/{parent}", app.HandleParent)
		r.Get("/op/{hash}/{parent}", app.HandleOp)

		r.Get("/domains/{domain}", app.HandleDomains)

		r.Get("/logout", app.HandleLogout)
		r.Get("/login", app.ShowLogin)
		r.Post("/login", app.HandleLogin)

		r.Get("/auth/{provider}", littr.HandleAuth)
		r.Get("/auth/{provider}/callback", littr.HandleCallback)
	})

	r.With(models.Loader).Route("/api", func(r chi.Router) {
		r.With(app.LoadSessionData).Get("/accounts/verify_credentials", api.HandleVerifyCredentials)

		r.Route("/accounts/{handle}", func (r chi.Router) {
			r.Use(api.AccountCtxt)

			r.Get("/", api.HandleAccount)
			r.With(api.LoadFiltersCtxt, api.ItemCollectionCtxt).Get("/{collection}", api.HandleAccountCollection)
			r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/{collection}/{hash}", api.HandleAccountCollectionItem)
		})

		r.Route("/{collection}", func (r chi.Router) {
			r.Use(api.ServiceCtxt)

			r.With(api.LoadFiltersCtxt, api.ItemCollectionCtxt).Get("/", api.HandleServiceCollection)
			r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/{hash}", api.HandleServiceCollectionItem)
		})

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			api.HandleError(w, r, http.StatusNotFound, errors.Errorf("%s not found", r.RequestURI))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			api.HandleError(w, r, http.StatusMethodNotAllowed, errors.Errorf("invalid %s request", r.Method))
		})
	})

	littr.Run(r, wait)
}
