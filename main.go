package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/processing"
	"github.com/writeas/go-nodeinfo"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/juju/errors"
	_ "github.com/lib/pq"

	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/api"
	"github.com/mariusor/littr.go/app/frontend"
	"github.com/mariusor/littr.go/app/log"
)

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
	app.Instance = app.New()

	db.Init(&app.Instance)
	front, err := frontend.Init(frontend.Config{
		Logger:      app.Instance.Logger.New(log.Ctx{"package": "frontend"}),
		SessionKeys: app.Instance.SessionKeys,
		Secure:      app.Instance.Secure,
		BaseURL:     app.Instance.BaseURL,
		HostName:    app.Instance.HostName,
	})
	if err != nil {
		app.Instance.Logger.Warn(err.Error())
	}
	// api
	host := os.Getenv("HOSTNAME")
	var apiURL string
	if app.Instance.Secure {
		apiURL = fmt.Sprintf("https://%s/api", host)
	} else {
		apiURL = fmt.Sprintf("http://%s/api", host)
	}
	a := api.Init(api.Config{
		Logger:      app.Instance.Logger.New(log.Ctx{"package": "api"}),
		BaseURL: apiURL,
	})
	//api.Config.BaseUrl = api.BaseURL

	processing.InitQueues(&app.Instance)

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	app.Logger = app.Instance.Logger.New(log.Ctx{"package": "app"})
	db.Logger = app.Instance.Logger.New(log.Ctx{"package": "db"})
	processing.Logger = app.Instance.Logger.New(log.Ctx{"package": "processing"})

	// Routes
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(app.ReqLogger)
	//r.Use(app.ShowHeaders)

	if app.Instance.Config.Env == app.PROD {
		r.Use(middleware.Recoverer)
	}

	cfg := nodeinfo.Config{
		BaseURL: api.BaseURL,
		InfoURL: "/nodeinfo",

		Metadata: nodeinfo.Metadata{
			NodeName:        app.Instance.NodeInfo().Title,
			NodeDescription: app.Instance.NodeInfo().Summary,
			Private:         false,
		},
		Protocols: []nodeinfo.NodeProtocol{
			nodeinfo.ProtocolActivityPub,
		},
		Services: nodeinfo.Services{
			Inbound: []nodeinfo.NodeService{},
			Outbound: []nodeinfo.NodeService{},
		},
		Software: nodeinfo.SoftwareInfo{
			Name:    app.Instance.NodeInfo().Title,
			Version: app.Instance.NodeInfo().Version,
		},
	}
	ni := nodeinfo.NewService(cfg, api.NodeInfoResolver{})
	// Frontend
	r.With(a.Repository).Route("/", func(r chi.Router) {
		r.Use(front.LoadSession)
		r.Use(middleware.GetHead)
		r.Use(app.NeedsDBBackend(front.HandleError))
		//r.Use(middleware.RedirectSlashes)

		r.Get("/", front.HandleIndex)

		r.Get("/about", front.HandleAbout)

		r.Get("/submit", front.ShowSubmit)
		r.Post("/submit", front.HandleSubmit)

		r.Get("/register", front.ShowRegister)
		r.Post("/register", front.HandleRegister)

		r.Route("/~{handle}", func(r chi.Router) {
			r.Get("/", front.ShowAccount)
			r.Get("/{hash}", front.ShowItem)
			r.Post("/{hash}", front.HandleSubmit)
			r.Get("/{hash}/{direction}", front.HandleVoting)
		})

		//r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/", frontend.HandleDate)
		r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", front.ShowItem)
		r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}/{direction}", front.HandleVoting)
		r.Post("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", front.HandleSubmit)

		r.Get("/item/{hash}", front.HandleItemRedirect)

		r.Get("/domains/{domain}", front.HandleDomains)
		r.Get("/tags/{tag}", front.HandleTags)

		r.With(front.NeedsSessions).Get("/logout", front.HandleLogout)
		r.With(front.NeedsSessions).Get("/login", front.ShowLogin)
		r.With(front.NeedsSessions).Post("/login", front.HandleLogin)

		r.Get("/self", front.HandleIndex)
		r.Get("/federated", front.HandleIndex)
		r.Get("/followed", front.HandleIndex)

		r.With(front.NeedsSessions).Get("/auth/{provider}", front.HandleAuth)
		r.With(front.NeedsSessions).Get("/auth/{provider}/callback", front.HandleCallback)

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			front.HandleError(w, r,errors.NotFoundf("%q not found", r.RequestURI))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			front.HandleError(w, r, errors.MethodNotAllowedf("invalid %q request", r.Method))
		})
	})

	// API
	r.With(db.Repository).Route("/api", func(r chi.Router) {
		//r.Use(middleware.GetHead)
		r.Use(a.VerifyHttpSignature)
		r.Use(app.StripCookies)
		r.Use(app.NeedsDBBackend(a.HandleError))

		r.Route("/self", func(r chi.Router) {
			r.With(api.LoadFiltersCtxt).Get("/", a.HandleService)
			r.Route("/{collection}", func(r chi.Router) {
				r.Use(api.ServiceCtxt)

				r.With(api.LoadFiltersCtxt, a.ItemCollectionCtxt).Get("/", a.HandleCollection)
				//r.With(api.LoadFiltersCtxt, a.LoadActivity).Post("/", a.AddToCollection)
				r.Route("/{hash}", func(r chi.Router) {
					r.With(api.LoadFiltersCtxt, a.ItemCtxt).Get("/", a.HandleCollectionActivity)
					r.With(api.LoadFiltersCtxt, a.ItemCtxt).Get("/object", a.HandleCollectionActivityObject)
					r.With(api.LoadFiltersCtxt, a.ItemCollectionCtxt).Get("/object/replies", a.HandleCollection)
				})
			})
		})
		r.Route("/actors", func(r chi.Router) {
			r.With(api.LoadFiltersCtxt).Get("/", a.HandleActorsCollection)

			r.Route("/{handle}", func(r chi.Router) {
				r.Use(a.AccountCtxt)

				r.Get("/", a.HandleActor)
				r.Route("/{collection}", func(r chi.Router) {
					r.With(api.LoadFiltersCtxt, a.ItemCollectionCtxt).Get("/", a.HandleCollection)
					r.With(api.LoadFiltersCtxt).Post("/", a.UpdateItem)
					r.Route("/{hash}", func(r chi.Router) {
						r.Use(middleware.GetHead)
						// this should update the activity
						r.With(api.LoadFiltersCtxt, a.ItemCtxt).Put("/", a.UpdateItem)
						r.With(api.LoadFiltersCtxt).Post("/", a.UpdateItem)
						r.With(api.LoadFiltersCtxt, a.ItemCtxt).Get("/", a.HandleCollectionActivity)
						r.With(api.LoadFiltersCtxt, a.ItemCtxt).Get("/object", a.HandleCollectionActivityObject)
						// this should update the item
						r.With(api.LoadFiltersCtxt, a.ItemCtxt).Put("/object", a.UpdateItem)
						r.With(api.LoadFiltersCtxt, a.ItemCtxt, a.ItemCollectionCtxt).Get("/object/replies", a.HandleCollection)
					})
				})
			})
		})

		// Mastodon compatible end-points
		r.Get("/v1/instance", a.ShowInstance)
		r.Get("/v1/instance/peers", api.ShowPeers)
		r.Get("/v1/instance/activity", api.ShowActivity)

		r.Get(cfg.InfoURL, ni.NodeInfo)

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			a.HandleError(w, r, errors.NotFoundf("%s not found", r.RequestURI))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			a.HandleError(w, r, errors.MethodNotAllowedf("invalid %s request", r.Method))
		})
	})

	// Web-Finger
	r.With(db.Repository).Route("/.well-known", func(r chi.Router) {
		r.Use(app.NeedsDBBackend(a.HandleError))

		r.Get("/webfinger", a.HandleWebFinger)
		r.Get("/host-meta", api.HandleHostMeta)
		r.Get("/nodeinfo", ni.NodeInfoDiscover)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			a.HandleError(w, r, errors.NotFoundf("%s not found", r.RequestURI))
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
	r.With(app.StripCookies).Get("/robots.txt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(assets, "robots.txt"))
	}))
	r.With(app.StripCookies).Get("/css/{path}", serveFiles(filepath.Join(assets, "css")))
	r.With(app.StripCookies).Get("/js/{path}", serveFiles(filepath.Join(assets, "js")))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		front.HandleError(w, r, errors.NotFoundf("%s not found", r.RequestURI))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		front.HandleError(w, r, errors.MethodNotAllowedf("%s not allowed", r.Method))
	})

	app.Instance.Run(r, wait)
}
