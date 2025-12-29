package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"git.sr.ht/~mariusor/brutalinks"
	"git.sr.ht/~mariusor/brutalinks/internal/config"
	log "git.sr.ht/~mariusor/lw"
	w "git.sr.ht/~mariusor/wrapper"
	"github.com/alecthomas/kong"
	"github.com/go-ap/errors"
)

var version = "HEAD"
var AppName = "brutalinks"

const defaultPort = config.DefaultListenPort
const defaultTimeout = time.Second * 5

// Run is the wrapper for starting the web-server and handling signals
func (s Serve) Run(cc ctl) error {
	c := cc.conf
	l := cc.logger
	a, err := brutalinks.New(c, l, s.Host, s.Port, version)
	if err != nil {
		l.Errorf("Failed to start application: %+s", err)
		os.Exit(1)
	}
	ctx, cancelFn := context.WithCancel(context.TODO())

	setters := []w.SetFn{w.Handler(a.Mux), w.GracefulWait(s.Wait)}
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

	l = l.WithContext(log.Ctx{
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
		}
		if err := srvStop(ctx); err != nil {
			l.Errorf("Stopped with error: %s", err)
		} else {
			l.Infof("Stopped")
		}
	}
	defer stopFn(ctx)

	l.Infof("Started")

	// Set up the signal handlers functions so the OS can tell us if it requires us to stop
	sigHandlerFns := w.SignalHandlers{
		syscall.SIGHUP: func(_ chan<- error) {
			l.Infof("SIGHUP received, reloading configuration")
			if err := a.Reload(); err != nil {
				l.Errorf("Failed to reload: %s", err.Error())
			}
		},
		syscall.SIGUSR1: func(_ chan<- error) {
			inMaintenanceMode := a.Conf.MaintenanceMode
			op := "to"
			if inMaintenanceMode {
				op = "out of"
			}
			l.Infof("SIGUSR1 received, switching %s maintenance mode", op)
			a.Conf.MaintenanceMode = !inMaintenanceMode
		},
		syscall.SIGTERM: func(exit chan<- error) {
			// kill -SIGTERM XXXX
			l.Infof("SIGTERM received, stopping")
			exit <- w.Interrupt
		},
		syscall.SIGINT: func(exit chan<- error) {
			// kill -SIGINT XXXX or Ctrl+c
			l.Infof("SIGINT received, stopping")
			exit <- w.Interrupt
		},
		syscall.SIGQUIT: func(exit chan<- error) {
			l.Warnf("SIGQUIT received, force stopping")
			cancelFn()
			exit <- w.Interrupt
		},
	}

	// Wait for OS signals asynchronously
	err = w.RegisterSignalHandlers(sigHandlerFns).Exec(ctx, srvRun)
	if err == nil {
		l.Infof("Shutting down")
	}
	return err
}

type CTL struct {
	Verbose int              `counter:"v" help:"Increase verbosity level from the default associated with the environment settings."`
	Path    string           `path:"" help:"The path for the storage folder or socket" default:"." env:"STORAGE_PATH"`
	Version kong.VersionFlag `short:"V"`

	// Commands
	Run Serve `cmd:"" help:"Run the ${name} instance server (version: ${version})" default:"withargs"`
}

type Serve struct {
	Wait time.Duration  `default:"${defaultTimeout}" help:"the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m"`
	Env  config.EnvType `enum:"${envTypes}" help:"The environment to use. Expected values: ${envTypes}" default:"${defaultEnv}"`
	Port int            `default:"${defaultPort}" help:"the port on which we should listen on"`
	Host string         `help:"the host on which we should listen on"`
}

type ctl struct {
	conf   *config.Configuration
	logger log.Logger
}

func main() {
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

	CTLRun := new(CTL)
	ctx := kong.Parse(
		CTLRun,
		kong.Name(AppName),
		kong.Description("${name} server (version ${version})"),
		kong.Vars{
			"defaultTimeout": defaultTimeout.String(),
			"version":        version,
			"name":           AppName,
			"defaultEnv":     string(config.DEV),
			"defaultPort":    strconv.Itoa(defaultPort),
			"envTypes":       fmt.Sprintf("%s, %s, %s, %s", config.TEST, config.DEV, config.QA, config.PROD),
		},
	)
	c := config.Load(CTLRun.Run.Env, CTLRun.Run.Wait)
	c.Version = version

	l := log.Dev(log.SetLevel(c.LogLevel))
	errors.SetIncludeBacktrace(c.Env.IsDev())

	if err := ctx.Run(ctl{conf: c, logger: l}); err != nil {
		l.WithContext(log.Ctx{"err": err}).Errorf("failed to run server")
		os.Exit(1)
	}
	os.Exit(0)
}
