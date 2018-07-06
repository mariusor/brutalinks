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

	"github.com/mariusor/littr.go/api"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/models"
)

const defaultHost = "littr.git"
const defaultPort = 3000

var listenHost = os.Getenv("HOSTNAME")
var listenPort, _ = strconv.ParseInt(os.Getenv("PORT"), 10, 64)

var littr app.Littr

var LocalUser = models.AnonymousAccount()

func CurrentAccount() *models.Account {
	return &LocalUser
}

func init() {
	authKey := []byte(os.Getenv("SESS_AUTH_KEY"))
	encKey := []byte(os.Getenv("SESS_ENC_KEY"))
	if listenPort == 0 {
		listenPort = defaultPort
	}
	if listenHost == "" {
		listenHost = defaultHost
	}

	s := sessions.NewCookieStore(authKey, encKey)
	s.Options.Domain = listenHost
	s.Options.Path = "/"

	littr = app.Littr{Host: listenHost, Port: listenPort, SessionStore: s}

	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Print(err)
	}

	littr.Db = db
}

func main() {
	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	m := mux.NewRouter()

	app.CurrentAccount = CurrentAccount()
	api.CurrentAccount = CurrentAccount()
	api.Db = littr.Db

	m.HandleFunc("/", littr.HandleIndex).
		Methods(http.MethodGet, http.MethodHead).
		Name("index")

	m.HandleFunc("/submit", littr.HandleSubmit).
		Methods(http.MethodGet, http.MethodHead, http.MethodPost).
		Name("submit")

	m.HandleFunc("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", littr.HandleContent).
		Methods(http.MethodGet, http.MethodHead, http.MethodPost).
		Name("content")

	//m.HandleFunc("/.well-known/webfinger", littr.handleWebFinger).
	//	Methods(http.MethodGet, http.MethodHead).
	//	Name("webfinger")

	m.HandleFunc("/~{handle}", littr.HandleUser).
		Methods(http.MethodGet, http.MethodHead).
		Name("account")

	o := m.PathPrefix("/auth").Subrouter()
	o.HandleFunc("/{provider}", littr.HandleAuth).Name("auth")
	o.HandleFunc("/{provider}/callback", littr.HandleCallback).Name("authCallback")

	ad := m.PathPrefix("/admin").Subrouter()
	ad.Use(littr.AuthCheck)
	ad.HandleFunc("/", littr.HandleAdmin).Name("admin")

	ap := m.PathPrefix("/api").Subrouter()
	ap.HandleFunc("/accounts/verify_credentials", api.HandleVerifyCredentials).Name("api-verify-credentials")
	ap.HandleFunc("/accounts/{handle}", api.HandleAccount).Name("api-account")

	m.PathPrefix("/assets/").
		Handler(http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets/"))))

	m.Handle("/favicon.ico", http.FileServer(http.Dir("./assets/")))

	m.HandleFunc("/{ancestor}/{hash}/{parent}", littr.HandleParent).
		Methods(http.MethodGet, http.MethodHead).
		Name("parent")

	m.HandleFunc("/domains/{domain}", littr.HandleDomains).
		Methods(http.MethodGet, http.MethodHead).
		Name("domains")

	m.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		littr.HandleError(w, r, fmt.Errorf("url %q couldn't be found", r.URL), 404)
	})

	m.Use(littr.LoggerMw)
	m.Use(littr.Sessions)

	littr.Run(m, wait)
}
