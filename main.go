package main

import (
	"crypto/tls"
	"flag"
	"net/http"
	"time"

	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	_ "github.com/lib/pq"

	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/internal/log"
)

var version = "HEAD"

const defaultPort = 3000
const defaultTimeout = time.Second * 15

func main() {
	var wait time.Duration
	var port int
	var host string
	var env string

	flag.DurationVar(&wait, "graceful-timeout", defaultTimeout, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.IntVar(&port, "port", defaultPort, "the port on which we should listen on")
	flag.StringVar(&host, "host", "", "the host on which we should listen on")
	flag.StringVar(&env, "env", "unknown", "the environment type")
	flag.Parse()

	e := app.EnvType(env)
	app.Instance = app.New(host, port, e, version)
	errors.IncludeBacktrace = app.Instance.Config.Env == app.DEV
	app.Logger = app.Instance.Logger.New(log.Ctx{"package": "app"})

	// Routes
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	if app.Instance.Config.Env == app.PROD {
		r.Use(middleware.Recoverer)
	} else {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	app.Instance.Front(r)
	app.Instance.Run(r, wait)
}
