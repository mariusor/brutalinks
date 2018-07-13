package main

import (
	"database/sql"
	"flag"
	"fmt"

	"log"
	"net/http"
	"os"
	"time"

	"strconv"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	_ "github.com/lib/pq"

	"encoding/gob"

	"github.com/mariusor/littr.go/api"
	"github.com/mariusor/littr.go/app"
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

func main() {
	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	m := mux.NewRouter()

	api.BaseURL = littr.Host

	m.HandleFunc("/", app.HandleIndexAPI).
		Methods(http.MethodGet, http.MethodHead).
		Name("index")

	m.HandleFunc("/submit", app.HandleSubmit).
		Methods(http.MethodGet, http.MethodHead, http.MethodPost).
		Name("submit")

	m.HandleFunc("/register", app.HandleRegister).
		Methods(http.MethodGet, http.MethodHead, http.MethodPost).
		Name("register")

	m.HandleFunc("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", app.HandleContent).
		Methods(http.MethodGet, http.MethodHead, http.MethodPost).
		Name("content")

	m.HandleFunc("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}/{direction}", app.HandleVoting).
		Methods(http.MethodGet, http.MethodHead).
		Name("content")

	//m.HandleFunc("/.well-known/webfinger", littr.handleWebFinger).
	//	Methods(http.MethodGet, http.MethodHead).
	//	Name("webfinger")

	m.HandleFunc("/~{handle}", app.HandleUser).
		Methods(http.MethodGet, http.MethodHead).
		Name("account")

	o := m.PathPrefix("/auth").Subrouter()
	o.HandleFunc("/local", app.HandleLogin).Name("login")
	o.HandleFunc("/{provider}", littr.HandleAuth).Name("auth")
	o.HandleFunc("/{provider}/callback", littr.HandleCallback).Name("authCallback")

	ad := m.PathPrefix("/admin").Subrouter()
	ad.Use(littr.AuthCheck)
	ad.HandleFunc("/", app.HandleAdmin).Name("admin")

	ap := m.PathPrefix("/api").Subrouter()
	ap.HandleFunc("/accounts/verify_credentials", api.HandleVerifyCredentials).Name("api-verify-credentials")
	ap.HandleFunc("/accounts/{handle}", api.HandleAccount).Name("api-account")
	ap.HandleFunc("/accounts/{handle}/{type}", api.HandleAccountOutbox).Name("api-account-child")

	m.PathPrefix("/assets/").
		Handler(http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets/"))))

	m.Handle("/favicon.ico", http.FileServer(http.Dir("./assets/")))

	m.HandleFunc("/{ancestor}/{hash}/{parent}", app.HandleParent).
		Methods(http.MethodGet, http.MethodHead).
		Name("parent")

	m.HandleFunc("/domains/{domain}", app.HandleDomains).
		Methods(http.MethodGet, http.MethodHead).
		Name("domains")

	m.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.HandleError(w, r, http.StatusNotFound, fmt.Errorf("url %q couldn't be found", r.URL))
	})

	m.Use(littr.LoggerMw)
	m.Use(littr.Sessions)

	littr.Run(m, wait)
}
