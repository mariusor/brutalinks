package main

import (
	"context"
	"crypto/tls"
	"flag"
	"github.com/go-ap/errors"
	"github.com/mariusor/littr.go/internal/config"
	"github.com/mariusor/littr.go/internal/log"
	"io/ioutil"
	golog "log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"

	"github.com/mariusor/littr.go/app"
)

var version = "HEAD"

const defaultPort = config.DefaultListenPort
const defaultTimeout = time.Second * 15

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
func SetupHttpServer(ctx context.Context, conf config.Configuration, m http.Handler) (func() error, func() error) {
	var serveFn func() error
	var srv *http.Server
	fileExists := func(dir string) bool {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return false
		}
		return true
	}

	srv = &http.Server{
		Addr:     conf.Listen(),
		Handler:  m,
		ErrorLog: golog.New(ioutil.Discard, "", 0),
	}
	if conf.Secure && fileExists(conf.CertPath) && fileExists(conf.KeyPath) {
		srv.TLSConfig = &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			},
		}
		serveFn = func() error {
			return srv.ListenAndServeTLS(conf.CertPath, conf.KeyPath)
		}
	} else {
		serveFn = srv.ListenAndServe
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
	return serveFn, shutdown
}

// Run is the wrapper for starting the web-server and handling signals
func Run(a app.Application, wait time.Duration) {
	a.Logger.WithContext(log.Ctx{
		"listen": a.Conf.Listen(),
		"host":   a.Conf.HostName,
		"env":    a.Conf.Env,
		"https":  a.Conf.Secure,
	}).Info("Started")

	srvStart, srvShutdown := SetupHttpServer(context.Background(), *a.Conf, a.Mux)
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
			a.Conf = config.Load(a.Conf.Env)
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

	c := config.Load(config.EnvType(env))
	errors.IncludeBacktrace = c.Env.IsDev()

	// Routes
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	if !c.Env.IsProd() {
		r.Use(middleware.Recoverer)
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	a := app.New(c, host, port,  version, r)
	Run(a, wait)
}
