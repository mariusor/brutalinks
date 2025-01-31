package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"git.sr.ht/~mariusor/brutalinks"
	"git.sr.ht/~mariusor/brutalinks/internal/config"
	log "git.sr.ht/~mariusor/lw"
	w "git.sr.ht/~mariusor/wrapper"
	"github.com/go-ap/errors"
)

var version = "HEAD"

const defaultPort = config.DefaultListenPort
const defaultTimeout = time.Second * 5

// Run is the wrapper for starting the web-server and handling signals
func Run(a *brutalinks.Application) error {
	ctx, cancelFn := context.WithCancel(context.TODO())

	setters := []w.SetFn{w.Handler(a.Mux)}
	if a.Conf.Secure && len(a.Conf.CertPath) > 0 && len(a.Conf.KeyPath) > 0 {
		setters = append(setters, w.WithTLSCert(a.Conf.CertPath, a.Conf.KeyPath))
	}
	if a.Conf.ListenHost == "systemd" {
		setters = append(setters, w.OnSystemd())
	} else if filepath.IsAbs(a.Conf.ListenHost) {
		dir, _ := filepath.Split(a.Conf.ListenHost)
		if _, err := os.Stat(dir); err == nil {
			setters = append(setters, w.OnSocket(a.Conf.ListenHost))
			defer func() { _ = os.RemoveAll(a.Conf.ListenHost) }()
		}
	} else {
		setters = append(setters, w.OnTCP(a.Conf.Listen()))
	}

	srvRun, srvStop := w.HttpServer(setters...)

	l := a.Logger.WithContext(log.Ctx{
		"version":  a.Version,
		"listenOn": a.Conf.Listen(),
		"TLS":      a.Conf.Secure,
		"host":     a.Conf.HostName,
		"env":      a.Conf.Env,
		"timeout":  a.Conf.TimeOut,
		"cert":     a.Conf.CertPath,
		"key":      a.Conf.KeyPath,
	})

	stopFn := func(ctx context.Context) {
		// NOTE(marius): close the storage repository
		if err := a.Close(); err != nil {
			l.Warnf("Close error: %s", err)
			return
		}
		if err := srvStop(ctx); err != nil {
			l.Errorf("Stopped with error: %s", err)
		} else {
			l.Infof("Stopped")
		}
	}

	l.Infof("Started")

	defer stopFn(ctx)
	// Set up the signal handlers functions so the OS can tell us if it requires us to stop
	sigHandlerFns := w.SignalHandlers{
		syscall.SIGHUP: func(_ chan<- error) {
			l.Infof("SIGHUP received, reloading configuration")
			if err := a.Reload(); err != nil {
				l.Errorf("Failed to reload: %s", err.Error())
			}
		},
		syscall.SIGUSR1: func(_ chan<- error) {
			l.Infof("SIGUSR1 received, switching to maintenance mode")
			a.Conf.MaintenanceMode = !a.Conf.MaintenanceMode
		},
		syscall.SIGTERM: func(status chan<- error) {
			// kill -SIGTERM XXXX
			l.Infof("SIGTERM received, stopping")
			status <- nil
		},
		syscall.SIGINT: func(status chan<- error) {
			// kill -SIGINT XXXX or Ctrl+c
			l.Infof("SIGINT received, stopping")
			status <- nil
		},
		syscall.SIGQUIT: func(status chan<- error) {
			l.Warnf("SIGQUIT received, force stopping")
			cancelFn()
			status <- nil
		},
	}

	// Wait for OS signals asynchronously
	err := w.RegisterSignalHandlers(sigHandlerFns).Exec(ctx, srvRun)
	if err != nil {
		l.Infof("Shutting down")
	}
	return err
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
	l := log.Dev(log.SetLevel(c.LogLevel))
	if c.Env.IsDev() {
		errors.IncludeBacktrace = c.Env.IsDev()
	}

	if build, ok := debug.ReadBuildInfo(); ok && version == "HEAD" {
		if build.Main.Version != "(devel)" {
			version = build.Main.Version
		}
		for _, bs := range build.Settings {
			if bs.Key == "vcs.revision" {
				version = bs.Value[:8]
			}
			if bs.Key == "vcs.modified" {
				version += "-git"
			}
		}
	}

	c.Version = version
	a, err := brutalinks.New(c, l, host, port, version)
	if err != nil {
		l.Errorf("Failed to start application: %+s", err)
		os.Exit(1)
	}
	if err = Run(a); err != nil {
		l.Errorf("Error: %+s", err)
		os.Exit(1)
	}
	os.Exit(0)
}
