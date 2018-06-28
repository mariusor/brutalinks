package main

import (
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"golang.org/x/net/context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
	"html/template"
	"golang.org/x/oauth2"
	"github.com/astaxie/beego/orm"
	_ "github.com/lib/pq"

)

const sessionName = "_s"
const listenHost = "myk.localdomain"
const templateDir = "templates/"

var app littr

type littr struct {
	Host    string
	Session sessions.Store
}

type errorModel struct {
	Status int
	Title string
	Error error
}

//func (l *littr) session(r *http.Request) *sessions.Session {
//	sess, err := l.Session.Get(r, sessionName)
//	if err != nil {
//		log.Printf("unable to load session")
//		return nil
//	}
//	return sess
//}

func (l *littr) Run(m *mux.Router, wait time.Duration) {
	log.SetPrefix(l.Host + " ")
	log.SetFlags(0)
	log.SetOutput(l)

	srv := &http.Server{
		Addr: l.Host + ":3000",
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      m,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
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
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	log.Println("Shutting down")
	os.Exit(0)
}

// Write is used to conform to the Logger interface
func (l *littr) Write(bytes []byte) (int, error) {
	return fmt.Printf("%s [%s] %s", time.Now().UTC().Format("2006-01-02 15:04:05.999"), "DEBUG", bytes)
}

// handleAdmin serves /admin request
func (l *littr) handleAdmin(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("done!!!"))
}

func (l *littr) handleError(w http.ResponseWriter, r *http.Request, err error) {
	d := errorModel{
		Status: http.StatusInternalServerError,
		Title: fmt.Sprintf("Error %d", http.StatusInternalServerError),
		Error: err,
	}

	log.Printf("%s %s Message: %q", r.Method, r.URL, d.Error)

	t, _ := template.New("error.html").ParseFiles(templateDir + "error.html")
	t.Execute(w, d)
}

// handleMain serves /auth/{provider}/callback request
func (l *littr) handleCallback(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	provider := vars["provider"]
	providerErr := vars["error"]
	if providerErr != "" {
		t, _ := template.New("error.html").ParseFiles(templateDir + "error.html")
		t.Execute(w, fmt.Errorf("%s error %s", provider, providerErr))
		return
	}
	code := vars["code"]
	if code == "" {
		t, _ := template.New("error.html").ParseFiles(templateDir + "error.html")
		t.Execute(w, fmt.Errorf("%s error: Empty authentication token", provider))
		return
	}

	s, err := l.Session.Get(r, sessionName)
	if err != nil {
		log.Printf("ERROR %s", err)
	}
	s.Values["auth_token"] = code
	s.AddFlash("Success")

	l.Session.Save(r, w, s)
}

// handleMain serves /auth/{provider}/callback request
func (l *littr) handleAuth(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	provider := vars["provider"]

	url, _ := mux.CurrentRoute(r).
		Subrouter().
		Get("authCallback").
		Host(listenHost + ":3000").
		URL("provider", provider)

	var config oauth2.Config
	switch provider {
	case "github":
		config = oauth2.Config{
			ClientID:     os.Getenv("GITHUB_KEY"),
			ClientSecret: os.Getenv("GITHUB_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL: 	"https://github.com/login/oauth/authorize",
				TokenURL:   "https://github.com/login/oauth/access_token",
			},
			RedirectURL: url.String(),
		}
	case "facebook":
		config = oauth2.Config{
			ClientID:     os.Getenv("FACEBOOK_KEY"),
			ClientSecret: os.Getenv("FACEBOOK_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL: "https://graph.facebook.com/oauth/authorize",
				TokenURL:     "https://graph.facebook.com/oauth/access_token",
			},
			RedirectURL:  url.String(),
		}
	case "google":
		config = oauth2.Config{
			ClientID:     os.Getenv("GOOGLE_KEY"),
			ClientSecret: os.Getenv("GOOGLE_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL: 	"https://accounts.google.com/o/oauth2/auth", // access_type=offline
				TokenURL:   "https://accounts.google.com/o/oauth2/token",
			},
			RedirectURL: url.String(),
		}
	default:
		s, err := l.Session.Get(r, sessionName)
		if err != nil {
			log.Printf("ERROR %s", err)
		}
		s.AddFlash("Missing oauth provider")
		indexUrl, _ := mux.CurrentRoute(r).Subrouter().Get("index").URL()
		http.Redirect(w, r, indexUrl.String(), http.StatusNotFound)
	}
	http.Redirect(w, r, config.AuthCodeURL("state", oauth2.AccessTypeOnline), http.StatusFound)
}

func (l *littr) loggerMw(n http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.RequestURI)
		n.ServeHTTP(w, r)
	})
}

func (l *littr) authCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		s, err := l.Session.Get(r, sessionName)
		if err != nil {
			log.Printf("ERROR %s", err)
		}
		//data := []byte(fmt.Sprintf("found test: %#v", s.Values))
		//w.Write(data)

		l.Session.Save(r, w, s)
	})
}

func init() {
	authKey := []byte(os.Getenv("SESS_AUTH_KEY"))
	encKey := []byte(os.Getenv("SESS_ENC_KEY"))
	s := sessions.NewCookieStore(authKey, encKey)
	s.Options.Domain = listenHost
	s.Options.Path = "/"

	app = littr{Host: listenHost, Session: s}

	dbPw := os.Getenv("DB_PASSWORD")
	orm.NewLog(&app)
	err := orm.RegisterDataBase("default", "postgres", "user=littr password="+dbPw+" dbname=littr sslmode=disable", 30)
	if err != nil {
		log.Print(err)
	}
}

func main() {
	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	dir := http.Dir("./assets/")
	f,e := dir.Open("css/main.css")
	if e == nil {
		defer f.Close()
	} else {
		log.Print(e)
	}
	m := mux.NewRouter()

	m.HandleFunc("/", app.handleIndex).
		Methods(http.MethodGet, http.MethodHead).
		Name("index")

	m.HandleFunc("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", app.handleContent).
		Methods(http.MethodGet, http.MethodHead).
		Name("content")

	m.HandleFunc("/p/{hash}/{parent}", app.handleParent).
		Methods(http.MethodGet, http.MethodHead).
		Name("parent")

	//m.HandleFunc("/.well-known/webfinger", app.handleWebFinger).
	//	Methods(http.MethodGet, http.MethodHead).
	//	Name("webfinger")

	m.HandleFunc("/~{handle}", app.handleUser).
		Methods(http.MethodGet, http.MethodHead).
		Name("account")

	o := m.PathPrefix("/auth").Subrouter()
	o.HandleFunc("/{provider}", app.handleAuth).Name("auth")
	o.HandleFunc("/{provider}/callback", app.handleCallback).Name("authCallback")

	a := m.PathPrefix("/admin").Subrouter()
	a.Use(app.authCheck)
	a.HandleFunc("/", app.handleAdmin).Name("admin")

	m.PathPrefix("/assets/").
		Handler(http.StripPrefix("/assets/", http.FileServer(dir)))

	m.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := errorModel{
			Status: http.StatusNotFound,
			Title: fmt.Sprintf("Not found"),
			Error: fmt.Errorf("url %q couldn't be found", r.URL),
		}

		w.WriteHeader(d.Status)
		log.Printf("%s %s Message: %s", r.Method, r.URL, d.Error)
		t, _ := template.New("error.html").ParseFiles(templateDir + "error.html")
		t.Execute(w, d)
	})

	m.Use(app.loggerMw)
	app.Run(m, wait)
}
