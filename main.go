package main

import (
	"database/sql"
	"encoding/gob"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/gorilla/sessions"
	"github.com/juju/errors"
	_ "github.com/lib/pq"
	"github.com/mariusor/littr.go/app/api"
	"github.com/mariusor/littr.go/app/frontend"
	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

const defaultHost = "localhost"
const defaultPort = 3000

var listenHost string
var listenPort int64
var listenOn string

var littr frontend.Littr

func loadEnv(l *frontend.Littr) (bool, error) {
	l.SessionKeys[0] = []byte(os.Getenv("SESS_AUTH_KEY"))
	l.SessionKeys[1] = []byte(os.Getenv("SESS_ENC_KEY"))

	listenHost = os.Getenv("HOSTNAME")
	listenPort, _ = strconv.ParseInt(os.Getenv("PORT"), 10, 64)
	listenOn = os.Getenv("LISTEN")

	env := frontend.EnvType(os.Getenv("ENV"))
	if !frontend.ValidEnv(env) {
		env = frontend.DEV
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
	littr = frontend.Littr{HostName: listenHost, Port: listenPort}

	loadEnv(&littr)

	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stdout)

	if littr.Env == frontend.PROD {
		log.SetLevel(log.WarnLevel)
	} else {
		log.SetLevel(log.DebugLevel)
	}
	gob.Register(models.Account{})
	gob.Register(frontend.Flash{})

	s := sessions.NewCookieStore(littr.SessionKeys[0], littr.SessionKeys[1])
	//s.Options.Domain = littr.HostName
	s.Options.Path = "/"
	frontend.SessionStore = s

	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.WithFields(log.Fields{}).Error(errors.NewErrWithCause(err, "failed to connect to the database"))
	}

	models.Config.DB = db
	api.Config.BaseUrl = os.Getenv("LISTEN")
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
	if littr.Env == frontend.PROD {
		r.Use(middleware.Recoverer)
	}

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		frontend.HandleError(w, r, http.StatusNotFound, errors.Errorf("%s not found", r.RequestURI))
	})

	workDir, _ := os.Getwd()
	filesDir := filepath.Join(workDir, "assets")
	FileServer(r, "/assets", http.Dir(filesDir))

	r.With(api.Repository).Route("/", func(r chi.Router) {
		r.Use(frontend.LoadSessionData)

		r.Get("/", frontend.HandleIndex)

		r.Get("/submit", frontend.ShowSubmit)
		r.Post("/submit", frontend.HandleSubmit)

		r.Get("/register", frontend.ShowRegister)
		r.Post("/register", frontend.HandleRegister)

		//r.With(Item).Get("/~{handle}/{hash}", frontend.ShowItem)
		r.Route("/~{handle}", func(r chi.Router) {
			r.Get("/", frontend.ShowAccount)
			r.Get("/{hash}", frontend.ShowItem)
			r.Post("/{hash}", frontend.HandleSubmit)
			r.Get("/{hash}/{direction}", frontend.HandleVoting)
		})

		//r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/", frontend.HandleDate)
		r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", frontend.ShowItem)
		r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}/{direction}", frontend.HandleVoting)
		r.Post("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", frontend.HandleSubmit)

		r.Get("/item/{hash}", frontend.HandleItemRedirect)

		r.Get("/domains/{domain}", frontend.HandleDomains)

		r.Get("/logout", frontend.HandleLogout)
		r.Get("/login", frontend.ShowLogin)
		r.Post("/login", frontend.HandleLogin)

		r.Get("/auth/{provider}", littr.HandleAuth)
		r.Get("/auth/{provider}/callback", littr.HandleCallback)
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		frontend.HandleError(w, r, http.StatusNotFound, errors.Errorf("%q not found", r.RequestURI))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		frontend.HandleError(w, r, http.StatusMethodNotAllowed, errors.Errorf("invalid %q request", r.Method))
	})

	r.With(models.Repository).Route("/api", func(r chi.Router) {
		r.Route("/accounts", func(r chi.Router) {
			r.With(api.LoadFiltersCtxt).Get("/", api.HandleAccountsCollection)

			r.With(frontend.LoadSessionData).Get("/accounts/verify_credentials", api.HandleVerifyCredentials)
			r.Route("/{handle}", func(r chi.Router) {
				r.Use(api.AccountCtxt)

				r.Get("/", api.HandleAccount)
				r.Route("/{collection}", func(r chi.Router) {
					r.With(api.LoadFiltersCtxt, api.ItemCollectionCtxt).Get("/", api.HandleCollection)
					r.Route("/{hash}", func(r chi.Router) {
						r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/", api.HandleCollectionItem)
						r.With(api.LoadFiltersCtxt).Put("/", api.UpdateItem)

						r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/replies", api.HandleItemReplies)
					})
				})
			})
		})

		r.Route("/{collection}", func(r chi.Router) {
			r.Use(api.ServiceCtxt)

			r.With(api.LoadFiltersCtxt, api.ItemCollectionCtxt).Get("/", api.HandleCollection)
			r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/{hash}", api.HandleCollectionItem)
			r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/{hash}/replies", api.HandleItemReplies)
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
