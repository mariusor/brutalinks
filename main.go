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
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/juju/errors"
	_ "github.com/lib/pq"

	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/api"
	"github.com/mariusor/littr.go/app/frontend"
	"github.com/mariusor/littr.go/internal/log"
)

var version = "HEAD"

const defaultHost = "localhost"
const defaultPort = 3000
const defaultTimeout = time.Second * 15

func main() {
	var wait time.Duration
	var port int
	var host string

	flag.DurationVar(&wait, "graceful-timeout", defaultTimeout, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.IntVar(&port, "port", defaultPort, "the port on which we should listen on")
	flag.StringVar(&host, "host", defaultHost, "the host on which we should listen on")
	flag.Parse()

	app.Instance = app.New(host, port, version)

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
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		if app.Instance.Secure {
			apiURL = fmt.Sprintf("https://%s/api", host)
		} else {
			apiURL = fmt.Sprintf("http://%s/api", host)
		}
	}
	a := api.Init(api.Config{
		Logger:  app.Instance.Logger.New(log.Ctx{"package": "api"}),
		BaseURL: apiURL,
	})
	app.Instance.APIURL = apiURL

	processing.InitQueues(&app.Instance)

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	app.Logger = app.Instance.Logger.New(log.Ctx{"package": "app"})
	db.Logger = app.Instance.Logger.New(log.Ctx{"package": "db"})
	processing.Logger = app.Instance.Logger.New(log.Ctx{"package": "processing"})

	middleware.DefaultLogger = middleware.RequestLogger(&middleware.DefaultLogFormatter{
		Logger: app.Logger,
	})
	// Routes
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(app.ReqLogger)
	//r.Use(app.ShowHeaders)

	if app.Instance.Config.Env == app.PROD {
		r.Use(middleware.Recoverer)
	}
	// Frontend
	r.With(a.Repository).Route("/", front.Routes())

	// API
	r.With(db.Repository).Route("/api", a.Routes())

	cfg := nodeinfo.Config{
		BaseURL: api.BaseURL,
		InfoURL: "/nodeinfo",

		Metadata: nodeinfo.Metadata{
			NodeName:        app.Instance.NodeInfo().Title,
			NodeDescription: app.Instance.NodeInfo().Summary,
			Private:         false,
			Software: nodeinfo.SoftwareMeta{
				GitHub:   "https://github.com/mariusor/littr.go",
				HomePage: "https://littr.me",
				Follow:   "mariusor@metalhead.club",
			},
		},
		Protocols: []nodeinfo.NodeProtocol{
			nodeinfo.ProtocolActivityPub,
		},
		Services: nodeinfo.Services{
			Inbound:  []nodeinfo.NodeService{},
			Outbound: []nodeinfo.NodeService{},
		},
		Software: nodeinfo.SoftwareInfo{
			Name:    app.Instance.NodeInfo().Title,
			Version: app.Instance.NodeInfo().Version,
		},
	}

	// Web-Finger
	r.With(db.Repository).Route("/.well-known", func(r chi.Router) {
		r.Use(app.NeedsDBBackend(a.HandleError))

		ni := nodeinfo.NewService(cfg, api.NodeInfoResolver{})
		r.Get("/webfinger", a.HandleWebFinger)
		r.Get("/host-meta", api.HandleHostMeta)
		r.Get("/nodeinfo", ni.NodeInfoDiscover)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			a.HandleError(w, r, errors.NotFoundf("%s", r.RequestURI))
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		front.HandleError(w, r, errors.NotFoundf("%s", r.RequestURI))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		front.HandleError(w, r, errors.MethodNotAllowedf("%s not allowed", r.Method))
	})

	app.Instance.Run(r, wait)
}
