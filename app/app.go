package app

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/mariusor/littr.go/app/models"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
)


const defaultHost = "localhost"
const defaultPort = 3000

const (
	Anonymous = "anonymous"
	AnonymousHash = models.Hash("77b7b7215e8d78452dc40da9efbb65fdc918c757844387aa0f88143762495c6b")
)

var listenHost string
var listenPort int64
var listenOn string

type EnvType string

const DEV EnvType = "dev"
const PROD EnvType = "prod"
const QA EnvType = "qa"

var validEnvTypes = []EnvType{
	DEV,
	PROD,
}

type Application struct {
	Version		string
	Env         EnvType
	HostName    string
	Port        int64
	Listen      string
	Db          *sql.DB
	SessionKeys [2][]byte
}

var Instance Application

func New() Application {
	app := Application{HostName: listenHost, Port: listenPort}
	loadEnv(&app)
	return app
}

func validEnv(s EnvType) bool {
	for _, k := range validEnvTypes {
		if k == s {
			return true
		}
	}
	return false
}

func (a *Application) listen() string {
	if len(a.Listen) > 0 {
		return a.Listen
	}
	var port string
	if a.Port != 0 {
		port = fmt.Sprintf(":%d", a.Port)
	}
	return fmt.Sprintf("%s%s", a.HostName, port)
}

func (a *Application) BaseUrl() string {
	return fmt.Sprintf("http://%s", a.HostName)
}

func loadEnv(l *Application) (bool, error) {
	l.SessionKeys[0] = []byte(os.Getenv("SESS_AUTH_KEY"))
	l.SessionKeys[1] = []byte(os.Getenv("SESS_ENC_KEY"))

	listenHost = os.Getenv("HOSTNAME")
	listenPort, _ = strconv.ParseInt(os.Getenv("PORT"), 10, 64)
	listenOn = os.Getenv("LISTEN")

	l.Version = os.Getenv("VERSION")
	env := EnvType(os.Getenv("ENV"))
	if !validEnv(env) {
		env = DEV
	}
	Instance.Env = env
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

func (a *Application) Run(m http.Handler, wait time.Duration) {
	log.WithFields(log.Fields{}).Infof("starting debug level %q", log.GetLevel().String())
	log.WithFields(log.Fields{}).Infof("listening on %s", a.listen())

	srv := &http.Server{
		Addr: a.listen(),
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      m,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.WithFields(log.Fields{}).Error(err)
			os.Exit(1)
		}
	}()

	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)
	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	log.RegisterExitHandler(cancel)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	log.WithFields(log.Fields{}).Infof("shutting down")
	os.Exit(0)
}
