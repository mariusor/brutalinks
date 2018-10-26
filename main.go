package main

import (
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mariusor/littr.go/app/db"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/juju/errors"
	_ "github.com/lib/pq"

	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/api"
	"github.com/mariusor/littr.go/app/frontend"
	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

var Logger log.FieldLogger

func init() {
	Logger = log.StandardLogger()
	app.Logger = Logger.WithField("package", "app")

	if app.Instance.Config.Env == app.PROD {
		log.SetLevel(log.WarnLevel)
	} else {
		log.SetFormatter(&log.TextFormatter{
			DisableColors:          false,
			DisableLevelTruncation: true,
			ForceColors:            true,
		})
		log.SetOutput(os.Stdout)
		log.SetLevel(log.DebugLevel)
	}

	app.Instance = app.New()
	frontend.InitSessionStore(app.Instance)
	//processing.InitQueues(app.Instance)

	api.Logger = Logger.WithField("package", "api")
	models.Logger = Logger.WithField("package", "models")
	db.Logger = Logger.WithField("package", "db")
	frontend.Logger = Logger.WithField("package", "frontend")
	//processing.Logger = Logger.WithField("package", "processing")

	db.Config.DB = app.Instance.Db
}

func serveFiles(st string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(chi.URLParam(r, "path"))
		fullPath := filepath.Join(st, path)
		http.ServeFile(w, r, fullPath)
	}
}

func main() {
	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	// Routes
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(app.ReqLogger)
	//r.Use(app.ShowHeaders)

	if app.Instance.Config.Env == app.PROD {
		r.Use(middleware.Recoverer)
	}

	// Frontend
	r.With(api.Repository).Route("/", func(r chi.Router) {
		r.Use(frontend.LoadSession)
		r.Use(middleware.GetHead)
		r.Use(frontend.Renderer)
		r.Use(app.NeedsDBBackend(frontend.HandleError))
		//r.Use(middleware.RedirectSlashes)

		r.Get("/", frontend.HandleIndex)

		r.Get("/about", frontend.HandleAbout)

		r.Get("/submit", frontend.ShowSubmit)
		r.Post("/submit", frontend.HandleSubmit)

		r.Get("/register", frontend.ShowRegister)
		r.Post("/register", frontend.HandleRegister)

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

		r.With(frontend.NeedsSessions).Get("/logout", frontend.HandleLogout)
		r.With(frontend.NeedsSessions).Get("/login", frontend.ShowLogin)
		r.With(frontend.NeedsSessions).Post("/login", frontend.HandleLogin)

		r.Get("/self", frontend.HandleIndex)
		r.Get("/federated", frontend.HandleIndex)
		r.Get("/followed", frontend.HandleIndex)

		r.With(frontend.NeedsSessions).Get("/auth/{provider}", frontend.HandleAuth)
		r.With(frontend.NeedsSessions).Get("/auth/{provider}/callback", frontend.HandleCallback)

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			frontend.HandleError(w, r, http.StatusNotFound, errors.Errorf("%q not found", r.RequestURI))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			frontend.HandleError(w, r, http.StatusMethodNotAllowed, errors.Errorf("invalid %q request", r.Method))
		})
	})

	// API
	r.With(db.Repository).Route("/api", func(r chi.Router) {
		//r.Use(middleware.GetHead)
		r.Use(api.VerifyHttpSignature)
		r.Use(app.StripCookies)
		r.Use(app.NeedsDBBackend(api.HandleError))

		r.Route("/self", func(r chi.Router) {
			r.With(api.LoadFiltersCtxt).Get("/", api.HandleService)
			r.Route("/{collection}", func(r chi.Router) {
				r.Use(api.ServiceCtxt)

				r.With(api.LoadFiltersCtxt, api.ItemCollectionCtxt).Get("/", api.HandleCollection)
				r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/{hash}", api.HandleCollectionActivity)
				r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/{hash}/object", api.HandleCollectionActivityObject)
				r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/{hash}/object/replies", api.HandleCollectionActivityObjectReplies)
			})
		})
		r.Route("/actors", func(r chi.Router) {
			r.With(api.LoadFiltersCtxt).Get("/", api.HandleActorsCollection)

			r.With(frontend.LoadSession).Get("/actors/verify_credentials", api.HandleVerifyCredentials)
			r.Route("/{handle}", func(r chi.Router) {
				r.Use(api.AccountCtxt)

				r.Get("/", api.HandleActor)
				r.Route("/{collection}", func(r chi.Router) {
					r.With(api.LoadFiltersCtxt, api.ItemCollectionCtxt).Get("/", api.HandleCollection)
					r.With(api.LoadFiltersCtxt).Post("/", api.UpdateItem)
					r.Route("/{hash}", func(r chi.Router) {
						r.Use(middleware.GetHead)
						// this should update the activity
						r.With(api.LoadFiltersCtxt, api.ItemCtxt).Put("/", api.UpdateItem)
						r.With(api.LoadFiltersCtxt).Post("/", api.UpdateItem)
						r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/", api.HandleCollectionActivity)
						r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/object", api.HandleCollectionActivityObject)
						// this should update the item
						r.With(api.LoadFiltersCtxt, api.ItemCtxt).Put("/object", api.UpdateItem)

						r.With(api.LoadFiltersCtxt, api.ItemCtxt).Get("/object/replies", api.HandleCollectionActivityObjectReplies)
					})
				})
			})
		})

		// Mastodon compatible end-points
		r.Get("/v1/instance", api.ShowInstance)
		r.Get("/v1/instance/peers", api.ShowPeers)
		r.Get("/v1/instance/activity", api.ShowActivity)

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			api.HandleError(w, r, http.StatusNotFound, errors.Errorf("%s not found", r.RequestURI))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			api.HandleError(w, r, http.StatusMethodNotAllowed, errors.Errorf("invalid %s request", r.Method))
		})
	})

	// Web-Finger
	r.With(db.Repository).Route("/.well-known", func(r chi.Router) {
		r.Use(app.NeedsDBBackend(api.HandleError))

		r.Get("/webfinger", api.HandleWebFinger)
		r.Get("/host-meta", api.HandleHostMeta)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			api.HandleError(w, r, http.StatusNotFound, errors.Errorf("%s not found", r.RequestURI))
		})
	})

	workDir, _ := os.Getwd()
	assets := filepath.Join(workDir, "assets")

	// static
	r.With(app.StripCookies).Get("/ns", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json+ld")
		http.ServeFile(w, r, filepath.Join(assets, "ns.json"))
	}))
	r.With(app.StripCookies).Get("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(assets, "favicon.ico"))
	}))
	r.With(app.StripCookies).Get("/css/{path}", serveFiles(filepath.Join(assets, "css")))
	r.With(app.StripCookies).Get("/js/{path}", serveFiles(filepath.Join(assets, "js")))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		frontend.HandleError(w, r, http.StatusNotFound, errors.Errorf("%s not found", r.RequestURI))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		frontend.HandleError(w, r, http.StatusMethodNotAllowed, errors.Errorf("%s not allowed", r.Method))
	})

	app.Instance.Run(r, wait)
}
