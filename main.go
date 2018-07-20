package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
	"strconv"
	"encoding/gob"
	"net/http"
	"strings"
	"path/filepath"
		"github.com/mariusor/littr.go/api"
	"github.com/mariusor/littr.go/app"
		"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/gorilla/sessions"
	_ "github.com/lib/pq"
)

const defaultHost = "littr.git"
const defaultPort = 3000

var listenHost = os.Getenv("HOSTNAME")
var listenPort, _ = strconv.ParseInt(os.Getenv("PORT"), 10, 64)

var littr app.Littr

func init() {
	authKey := []byte(os.Getenv("SESS_AUTH_KEY"))
	encKey := []byte(os.Getenv("SESS_ENC_KEY"))
	if listenPort == 0 {
		listenPort = defaultPort
	}
	if listenHost == "" {
		listenHost = defaultHost
	}

	gob.Register(app.Account{})
	gob.Register(app.Flash{})
	s := sessions.NewCookieStore(authKey, encKey)
	//s.Options.Domain = listenHost
	s.Options.Path = "/"

	app.SessionStore = s

	littr = app.Littr{Host: listenHost, Port: listenPort}

	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Print(err)
	}

	api.Db = db
	app.Db = db
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

	// Routes
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(littr.Sessions)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		app.HandleError(w, r, http.StatusNotFound, fmt.Errorf("%s not found", r.RequestURI))
	})

	workDir, _ := os.Getwd()
	filesDir := filepath.Join(workDir, "assets")
	FileServer(r, "/assets", http.Dir(filesDir))

	r.Get("/", app.HandleIndex)

	r.Get("/submit", app.ShowSubmit)
	r.Post("/submit", app.HandleSubmit)

	r.Get("/register", app.ShowRegister)
	r.Post("/register", app.HandleRegister)

	r.Get("/~{handle}", app.HandleUser)

	r.Get("/~{handle}/{hash}", app.ShowContent)
	r.Post("/~{handle}/{hash}", app.HandleSubmit)
	r.Get("/~{handle}/{hash}/{direction}", app.HandleVoting)

	//r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/", app.HandleDate)
	r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", app.ShowContent)
	r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}/{direction}", app.HandleVoting)
	r.Post("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", app.HandleSubmit)

	r.Get("/parent/{hash}/{parent}", app.HandleParent)
	r.Get("/op/{hash}/{parent}", app.HandleOp)

	r.Get("/domains/{domain}", app.HandleDomains)

	r.Get("/logout", app.HandleLogout)
	r.Get("/login", app.ShowLogin)
	r.Post("/login", app.HandleLogin)

	r.Get("/auth/{provider}", littr.HandleAuth)
	r.Get("/auth/{provider}/callback", littr.HandleCallback)

	r.Route("/api", func(r chi.Router) {
		r.Get("/accounts/verify_credentials", api.HandleVerifyCredentials)
		r.Get("/accounts/{handle}", api.HandleAccount)
		r.Get("/accounts/{handle}/{path}", api.HandleAccountPath)

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			api.HandleError(w, r, http.StatusNotFound, fmt.Errorf("%s not found", r.RequestURI))
		})
	})

	littr.Run(r, wait)
}
