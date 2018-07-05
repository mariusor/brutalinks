package main

import (
	"database/sql"
	"flag"
	"fmt"

	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	_ "github.com/lib/pq"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"math"
	"models"
	"strconv"
	"strings"
)

const sessionName = "_s"
const templateDir = "templates/"
const defaultHost = "myk.localdomain"
const defaultPort = 3000

var listenHost = os.Getenv("HOSTNAME")
var listenPort, _ = strconv.ParseInt(os.Getenv("PORT"), 10, 64)
var app littr

var LocalUser = models.AnonymousAccount()

func CurrentAccount() *models.Account {
	return &LocalUser
}

func LoadFlashMessages() []interface{} {
	return app.FlashData
}

func CleanFlashMessages() string {
	app.FlashData = app.FlashData[:0]
	return ""
}

func getAllIds(c []models.Content) []int64 {
	return models.ContentCollection(c).GetAllIds()
}

type littr struct {
	Host          string
	Port          int64
	Db            *sql.DB
	session       sessions.Store
	FlashData     []interface{}
	SessionData   []interface{}
	InvertedTheme bool
}

type errorModel struct {
	Status        int
	Title         string
	InvertedTheme bool
	Error         error
}

func (l *littr) Session(r *http.Request) *sessions.Session {
	s, err := l.session.Get(r, sessionName)
	if err != nil {
		log.Print(err)
	}
	return s
}

func (l *littr) host() string {
	var port string
	if l.Port != 0 {
		port = fmt.Sprintf(":%d", l.Port)
	}
	return fmt.Sprintf("%s%s", l.Host, port)
}

func (l *littr) BaseUrl() string {
	return fmt.Sprintf("http://%s", l.host())
}

func (l *littr) LoadVotes(u *models.Account, ids []int64) error {
	db := l.Db
	// this here code following is the ugliest I wrote in quite a long time
	// so ugly it warrants its own fucking shame corner
	sids := make([]string, 0)
	for i := 0; i < len(ids); i++ {
		sids = append(sids, fmt.Sprintf("$%d", i+2))
	}
	iitems := make([]interface{}, len(ids)+1)
	iitems[0] = u.Id
	for i, v := range ids {
		iitems[i+1] = v
	}
	sel := fmt.Sprintf(`select "id", "submitted_by", "submitted_at", "updated_at", "item_id", "weight", "flags"
	from "votes" where "submitted_by" = $1 and "item_id" in (%s)`, strings.Join(sids, ", "))
	rows, err := db.Query(sel, iitems...)
	if err != nil {
		return err
	}
	for rows.Next() {
		v := models.Vote{}
		err = rows.Scan(&v.Id, &v.SubmittedBy, &v.SubmittedAt, &v.UpdatedAt,
			&v.ItemId, &v.Weight, &u.Flags)
		if err != nil {
			return err
		}
		u.Votes[v.Id] = v
	}

	return nil
}

//func (l *littr) session(r *http.Request) *sessions.Session {
//	sess, err := l.Session.Get(r, sessionName)
//	if err != nil {
//		log.Printf("unable to load session")
//		return nil
//	}
//	return sess
//}

func (l *littr) Vote(p models.Content, score int, userId int64) (bool, error) {
	db := l.Db
	newWeight := int(score * models.ScoreMultiplier)

	var sel string
	var p2 interface{}
	if p.Id == 0 {
		sel = `select "id", "weight" from "votes" where "submitted_by" = $1 and "key" ~* $2;`
		p2 = interface{}(p.Key)
	} else {
		sel = `select "id", "weight" from "votes" where "submitted_by" = $1 and "item_id" = $2;`
		p2 = interface{}(p.Id)
	}

	v := models.Vote{}
	{
		rows, err := db.Query(sel, userId, p2)
		if err != nil {
			return false, err
		}
		for rows.Next() {
			err = rows.Scan(&v.Id, &v.Weight)
			if err != nil {
				return false, err
			}
		}
	}

	var q string
	if v.Id != 0 {
		if v.Weight != 0 && math.Signbit(float64(newWeight)) == math.Signbit(float64(v.Weight)) {
			newWeight = 0
		}
		q = `update "votes" set "updated_at" = now(), "weight" = $1 where "item_id" = $2 and "submitted_by" = $3;`
	} else {
		q = `insert into "votes" ("weight", "item_id", "submitted_by") values ($1, $2, $3)`
	}
	{
		res, err := db.Exec(q, newWeight, p.Id, userId)
		if err != nil {
			return false, err
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			return false, fmt.Errorf("scoring %d failed on item %q", newWeight, p.Hash())
		}
		log.Printf("%d scoring %d on %s", userId, newWeight, p.Hash())
	}

	upd := `update "content_items" set score = score - $1 + $2 where "id" = $3`
	{
		res, err := db.Exec(upd, v.Weight, newWeight, p.Id)
		if err != nil {
			return false, err
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			return false, fmt.Errorf("content hash %q not found", p.Hash())
		}
		if rows, _ := res.RowsAffected(); rows > 1 {
			return false, fmt.Errorf("content hash %q collision", p.Hash())
		}
		log.Printf("updated content_items with %d", newWeight)
	}

	return true, nil
}

func (l *littr) Run(m *mux.Router, wait time.Duration) {
	log.SetPrefix(l.Host + " ")
	log.SetFlags(0)
	log.SetOutput(l)

	srv := &http.Server{
		Addr: l.host(),
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

func (l *littr) handleError(w http.ResponseWriter, r *http.Request, err error, status int) {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	d := errorModel{
		Status:        status,
		Title:         fmt.Sprintf("Error %d", status),
		InvertedTheme: l.InvertedTheme,
		Error:         err,
	}
	w.WriteHeader(status)

	var terr error
	log.Printf("%s %s Message: %q", r.Method, r.URL, d.Error)
	t, terr := template.New("error.html").ParseFiles(templateDir + "error.html")
	t.Funcs(template.FuncMap{
		"getProviders":   getAuthProviders,
		"CurrentAccount": CurrentAccount,
	})
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("head.html").ParseFiles(templateDir + "partials/head.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("header.html").ParseFiles(templateDir + "partials/header.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("footer.html").ParseFiles(templateDir + "partials/footer.html")
	if terr != nil {
		log.Print(terr)
	}
	terr = t.Execute(w, d)
	if terr != nil {
		log.Print(terr)
	}
}

// handleMain serves /auth/{provider}/callback request
func (l *littr) handleCallback(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	q := r.URL.Query()
	provider := vars["provider"]
	providerErr := q["error"]
	if providerErr != nil {
		t, _ := template.New("error.html").ParseFiles(templateDir + "error.html")
		t.Execute(w, fmt.Errorf("%s error %s", provider, providerErr))
		return
	}
	code := q["code"]
	state := q["state"]
	if code == nil {
		t, _ := template.New("error.html").ParseFiles(templateDir + "error.html")
		t.Execute(w, fmt.Errorf("%s error: Empty authentication token", provider))
		return
	}

	s, err := l.session.Get(r, sessionName)
	if err != nil {
		log.Printf("ERROR %s", err)
	}

	s.Values["provider"] = provider
	s.Values["code"] = code
	s.Values["state"] = state
	s.AddFlash("Success")

	err = l.session.Save(r, w, s)
	if err != nil {
		log.Print(err)
	}
	http.Redirect(w, r, l.BaseUrl(), http.StatusFound)
}

// handleMain serves /auth/{provider}/callback request
func (l *littr) handleAuth(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	provider := vars["provider"]

	url := fmt.Sprintf("%s/auth/%s/callback", l.BaseUrl(), provider)

	var config oauth2.Config
	switch provider {
	case "github":
		config = oauth2.Config{
			ClientID:     os.Getenv("GITHUB_KEY"),
			ClientSecret: os.Getenv("GITHUB_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://github.com/login/oauth/authorize",
				TokenURL: "https://github.com/login/oauth/access_token",
			},
			RedirectURL: url,
		}
	case "gitlab":
		config = oauth2.Config{
			ClientID:     os.Getenv("GITLAB_KEY"),
			ClientSecret: os.Getenv("GITLAB_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://gitlab.com/login/oauth/authorize",
				TokenURL: "https://gitlab.com/login/oauth/access_token",
			},
			RedirectURL: url,
		}
	case "facebook":
		config = oauth2.Config{
			ClientID:     os.Getenv("FACEBOOK_KEY"),
			ClientSecret: os.Getenv("FACEBOOK_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://graph.facebook.com/oauth/authorize",
				TokenURL: "https://graph.facebook.com/oauth/access_token",
			},
			RedirectURL: url,
		}
	case "google":
		config = oauth2.Config{
			ClientID:     os.Getenv("GOOGLE_KEY"),
			ClientSecret: os.Getenv("GOOGLE_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/auth", // access_type=offline
				TokenURL: "https://accounts.google.com/o/oauth2/token",
			},
			RedirectURL: url,
		}
	default:
		s, err := l.session.Get(r, sessionName)
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
func (l *littr) sessions(n http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := l.session.Get(r, sessionName)
		if err != nil {
			log.Print(err)
		}
		l.FlashData = s.Flashes()

		cookies := r.Cookies()
		for _, c := range cookies {
			//log.Printf("cookie %s:%s", c.Name, c.Value)
			if c.Name == "inverted" {
				l.InvertedTheme = true
				break
			}
			l.InvertedTheme = false
		}
		n.ServeHTTP(w, r)
	})
}

func (l *littr) authCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		s, err := l.session.Get(r, sessionName)
		if err != nil {
			log.Printf("ERROR %s", err)
		}
		log.Printf("%#v", s.Values)
		//l.session.Save(r, w, s)
	})
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

	app = littr{Host: listenHost, Port: listenPort, session: s}

	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Print(err)
	}
	app.Db = db
}

func main() {
	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	m := mux.NewRouter()

	m.HandleFunc("/", app.handleIndex).
		Methods(http.MethodGet, http.MethodHead).
		Name("index")

	m.HandleFunc("/submit", app.handleSubmit).
		Methods(http.MethodGet, http.MethodHead, http.MethodPost).
		Name("submit")

	m.HandleFunc("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", app.handleContent).
		Methods(http.MethodGet, http.MethodHead, http.MethodPost).
		Name("content")

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
		Handler(http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets/"))))

	m.HandleFunc("/{ancestor}/{hash}/{parent}", app.handleParent).
		Methods(http.MethodGet, http.MethodHead).
		Name("parent")

	m.HandleFunc("/domains/{domain}", app.handleDomains).
		Methods(http.MethodGet, http.MethodHead).
		Name("domains")

	m.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := errorModel{
			Status:        http.StatusNotFound,
			Title:         fmt.Sprintf("Not found"),
			InvertedTheme: app.InvertedTheme,
			Error:         fmt.Errorf("url %q couldn't be found", r.URL),
		}

		w.WriteHeader(d.Status)
		log.Printf("%s %s Message: %s", r.Method, r.URL, d.Error)
		t, terr := template.New("error.html").ParseFiles(templateDir + "error.html")
		t.Funcs(template.FuncMap{
			"getProviders":   getAuthProviders,
			"CurrentAccount": CurrentAccount,
		})
		if terr != nil {
			log.Print(terr)
		}
		_, terr = t.New("footer.html").ParseFiles(templateDir + "partials/footer.html")
		if terr != nil {
			log.Print(terr)
		}
		_, terr = t.New("header.html").ParseFiles(templateDir + "partials/header.html")
		if terr != nil {
			log.Print(terr)
		}
		_, terr = t.New("head.html").ParseFiles(templateDir + "partials/head.html")
		if terr != nil {
			log.Print(terr)
		}
		t.Execute(w, d)
	})

	m.Use(app.loggerMw)
	m.Use(app.sessions)

	app.Run(m, wait)
}
