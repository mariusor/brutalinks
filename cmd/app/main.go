package main

import (
	"context"
	"flag"
	"os"
	"syscall"
	"time"

	w "git.sr.ht/~mariusor/wrapper"
	"github.com/go-ap/errors"
	"github.com/mariusor/go-littr/app"
	"github.com/mariusor/go-littr/internal/config"
	"github.com/mariusor/go-littr/internal/log"
)

var version = "HEAD"

const defaultPort = config.DefaultListenPort
const defaultTimeout = time.Second * 5

// Run is the wrapper for starting the web-server and handling signals
func Run(a app.Application) int {
	ctx, cancelFn := context.WithCancel(context.TODO())

	setters := []w.SetFn{w.Handler(a.Mux), w.ListenOn(a.Conf.Listen())}
	if a.Conf.Secure && len(a.Conf.CertPath) > 0 && len(a.Conf.KeyPath) > 0 {
		setters = append(setters, w.SSL(a.Conf.CertPath, a.Conf.KeyPath))
	}
	srvRun, srvStop := w.HttpServer(ctx, setters...)

	defer func() {
		cancelFn()
		if err := srvStop(); err != nil {
			a.Logger.Errorf("Error: %s", err)
		}
	}()

	runFn := func() error {
		// Run our server in a goroutine so that it doesn't block.
		if err := srvRun(); err != nil {
			a.Logger.Errorf("Error: %s", err)
			return err
		}
		return nil
	}

	a.Logger.WithContext(log.Ctx{
		"listen":  a.Conf.Listen(),
		"host":    a.Conf.HostName,
		"env":     a.Conf.Env,
		"https":   a.Conf.Secure,
		"timeout": a.Conf.TimeOut,
		"cert":    a.Conf.CertPath,
		"key":     a.Conf.KeyPath,
	}).Info("Started")

	// Set up the signal handlers functions so the OS can tell us if the it requires us to stop
	sigHandlerFns := w.SignalHandlers{
		syscall.SIGHUP: func(_ chan int) {
			a.Logger.Info("SIGHUP received, reloading configuration")
			a.Conf = config.Load(a.Conf.Env, a.Conf.TimeOut)
		},
		syscall.SIGUSR1: func(_ chan int) {
			a.Logger.Info("SIGUSR1 received, switching to maintenance mode")
			a.Conf.MaintenanceMode = !a.Conf.MaintenanceMode
		},
		syscall.SIGTERM: func(status chan int) {
			// kill -SIGTERM XXXX
			a.Logger.Info("SIGTERM received, stopping")
			status <- 0
		},
		syscall.SIGINT: func(status chan int) {
			// kill -SIGINT XXXX or Ctrl+c
			a.Logger.Info("SIGINT received, stopping")
			status <- 0
		},
		syscall.SIGQUIT: func(status chan int) {
			a.Logger.Error("SIGQUIT received, force stopping")
			status <- -1
		},
	}

	// Wait for OS signals asynchronously
	code := w.RegisterSignalHandlers(sigHandlerFns).Exec(runFn)
	if code == 0 {
		a.Logger.Info("Shutting down")
	}
	return code
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

	c := config.Load(config.EnvType(env), wait)
	errors.IncludeBacktrace = c.Env.IsDev()

	os.Exit(Run(app.New(c, host, port, version)))
}
