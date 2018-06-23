package main

import (
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"golang.org/x/net/context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
	"html/template"
)

const sessionName = "_s"
const listenHost = "myk.localdomain"
const templateDir = "templates/"

type littr struct {
	Host    string
	Session sessions.Store
}

func (l *littr) session(r *http.Request) *sessions.Session {
	sess, err := l.Session.Get(r, sessionName)
	if err != nil {
		log.Printf("unable to load session")
		return nil
	}
	return sess
}

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

// handleMain serves / request
func (l *littr) handleMain(w http.ResponseWriter, _ *http.Request) {
	m := make(map[string]string)
	m["github"] = "Github"

	t, _ := template.New("index.html").ParseFiles(templateDir + "index.html")
	t.Execute(w, m)
}

func (l *littr) handleAuth(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	provider := vars["provider"]
	switch provider {
	case "github":
	case "facebook":
	case "google":
	}

	sess := l.session(r)
	//sess.Values["test"] = "ana"

	if r != nil {
		sess.Save(r, w)
	}
}

func (l *littr) loggerMw(n http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.RequestURI)
		n.ServeHTTP(w, r)
	})
}

func (l *littr) AuthCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := l.Session.Get(r, sessionName)
		if err != nil {
			log.Printf("ERROR %s", err)
		}
		//data := []byte(fmt.Sprintf("found test: %#v", s.Values))
		//w.Write(data)

		l.Session.Save(r, w, s)
		next.ServeHTTP(w, r)
	})
}

func main() {
	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	s := sessions.NewCookieStore(securecookie.GenerateRandomKey(12))
	s.Options.Domain = listenHost
	s.Options.Path = "/"

	app := littr{Host: listenHost, Session: s}

	m := mux.NewRouter()
	m.HandleFunc("/", app.handleMain)
	m.HandleFunc("/auth/{provider}", app.handleAuth)

	a := m.PathPrefix("/admin").Subrouter()
	a.Use(app.AuthCheck)
	a.HandleFunc("/", app.handleAdmin)

	m.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//w.WriteHeader(404)
		d := struct {
			Title string
			Error error
		}{
			Title: fmt.Sprintf("Error %d", 404),
			Error: fmt.Errorf("not found: '%s'", r.RequestURI),
		}

		t, _ := template.New("error.html").ParseFiles(templateDir + "error.html")
		t.Execute(w, d)
		log.Printf("%s %s Message: %q", r.Method, r.RequestURI, d.Error)
	})

	m.Use(app.loggerMw)
	app.Run(m, wait)
}
