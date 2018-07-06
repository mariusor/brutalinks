package app

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/mariusor/littr.go/models"
	"golang.org/x/oauth2"
)

const sessionName = "_s"
const templateDir = "templates/"

var CurrentAccount *models.Account

type Littr struct {
	Host          string
	Port          int64
	Db            *sql.DB
	SessionStore  sessions.Store
	FlashData     []interface{}
	SessionData   []interface{}
	InvertedTheme bool
	ErrorHandler  http.HandlerFunc
}

type errorModel struct {
	Status        int
	Title         string
	InvertedTheme bool
	Error         error
}

func (l *Littr) GetSession(r *http.Request) *sessions.Session {
	s, err := l.SessionStore.Get(r, sessionName)
	if err != nil {
		log.Print(err)
	}
	return s
}

func (l *Littr) host() string {
	var port string
	if l.Port != 0 {
		port = fmt.Sprintf(":%d", l.Port)
	}
	return fmt.Sprintf("%s%s", "127.0.0.1", port)
}

func (l *Littr) BaseUrl() string {
	return fmt.Sprintf("http://%s", l.host())
}

func (l *Littr) LoadVotes(u *models.Account, ids []int64) error {
	if u == nil {
		return fmt.Errorf("invalid user")
	}
	if len(ids) == 0 {
		return fmt.Errorf("no ids to load")
	}
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

func RenderTemplate(w http.ResponseWriter, name string, m interface{}) error {
	t, terr := LoadTemplates(templateDir, name)
	if terr != nil {
		log.Print(terr)
		return terr
	}
	terr = t.Execute(w, m)
	if terr != nil {
		log.Print(terr)
		return terr
	}
	return nil
}

func LoadTemplates(base string, main string) (*template.Template, error) {
	var terr error
	var t *template.Template
	t, terr = template.New(main).ParseFiles(base + main)
	if terr != nil {
		return nil, terr
	}

	t.Funcs(template.FuncMap{
		"sluggify":           sluggify,
		"title":              func(t []byte) string { return string(t) },
		"getProviders":       getAuthProviders,
		"CurrentAccount":     func() *models.Account { return CurrentAccount },
		"LoadFlashMessages":  func() []int { return []int{} }, //LoadFlashMessages,
		"mod":                func(lvl int) float64 { return math.Mod(float64(lvl), float64(10)) },
		"CleanFlashMessages": func() string { return "" }, //CleanFlashMessages,
	})
	_, terr = t.New("items.html").ParseFiles(base + "partials/content/items.html")
	if terr != nil {
		return nil, terr
	}
	_, terr = t.New("link.html").ParseFiles(base + "partials/content/link.html")
	if terr != nil {
		return nil, terr
		log.Print(terr)
	}
	_, terr = t.New("submit.html").ParseFiles(templateDir + "partials/content/submit.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("comments.html").ParseFiles(templateDir + "partials/content/comments.html")
	if terr != nil {
		return nil, terr
		log.Print(terr)
	}
	_, terr = t.New("comment.html").ParseFiles(templateDir + "partials/content/comment.html")
	if terr != nil {
		return nil, terr
		log.Print(terr)
	}
	_, terr = t.New("flash.html").ParseFiles(base + "partials/flash.html")
	if terr != nil {
		return nil, terr
		log.Print(terr)
	}
	_, terr = t.New("meta.html").ParseFiles(base + "partials/content/meta.html")
	if terr != nil {
		return nil, terr
		log.Print(terr)
	}
	_, terr = t.New("data.html").ParseFiles(base + "partials/content/data.html")
	if terr != nil {
		return nil, terr
		log.Print(terr)
	}
	_, terr = t.New("score.html").ParseFiles(base + "partials/content/score.html")
	if terr != nil {
		return nil, terr
		log.Print(terr)
	}
	_, terr = t.New("head.html").ParseFiles(base + "partials/head.html")
	if terr != nil {
		return nil, terr
		log.Print(terr)
	}
	_, terr = t.New("header.html").ParseFiles(base + "partials/header.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("footer.html").ParseFiles(base + "partials/footer.html")
	if terr != nil {
		return nil, terr
		log.Print(terr)
	}
	return t, nil
}

//func (l *Littr) SessionStore(r *http.Request) *sessions.GetSession {
//	sess, err := l.GetSession.Get(r, sessionName)
//	if err != nil {
//		log.Printf("unable to load SessionStore")
//		return nil
//	}
//	return sess
//}

func (l *Littr) Vote(p models.Content, score int, userId int64) (bool, error) {
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

func (l *Littr) Run(m *mux.Router, wait time.Duration) {
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
func (l *Littr) Write(bytes []byte) (int, error) {
	return fmt.Printf("%s [%s] %s", time.Now().UTC().Format("2006-01-02 15:04:05.999"), "DEBUG", bytes)
}

// handleAdmin serves /admin request
func (l *Littr) HandleAdmin(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("done!!!"))
}

// handleMain serves /auth/{provider}/callback request
func (l *Littr) HandleCallback(w http.ResponseWriter, r *http.Request) {
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

	s, err := l.SessionStore.Get(r, sessionName)
	if err != nil {
		log.Printf("ERROR %s", err)
	}

	s.Values["provider"] = provider
	s.Values["code"] = code
	s.Values["state"] = state
	s.AddFlash("Success")

	err = l.SessionStore.Save(r, w, s)
	if err != nil {
		log.Print(err)
	}
	http.Redirect(w, r, l.BaseUrl(), http.StatusFound)
}

// handleMain serves /auth/{provider}/callback request
func (l *Littr) HandleAuth(w http.ResponseWriter, r *http.Request) {
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
		s, err := l.SessionStore.Get(r, sessionName)
		if err != nil {
			log.Printf("ERROR %s", err)
		}
		s.AddFlash("Missing oauth provider")
		indexUrl, _ := mux.CurrentRoute(r).Subrouter().Get("index").URL()
		http.Redirect(w, r, indexUrl.String(), http.StatusNotFound)
	}
	http.Redirect(w, r, config.AuthCodeURL("state", oauth2.AccessTypeOnline), http.StatusFound)
}

func (l *Littr) LoggerMw(n http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.RequestURI)
		n.ServeHTTP(w, r)
	})
}
func (l *Littr) Sessions(n http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := l.SessionStore.Get(r, sessionName)
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

func (l *Littr) AuthCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		s, err := l.SessionStore.Get(r, sessionName)
		if err != nil {
			log.Printf("ERROR %s", err)
		}
		log.Printf("%#v", s.Values)
		//l.SessionStore.Save(r, w, s)
	})
}

func getAllIds(c []models.Content) []int64 {
	return models.ContentCollection(c).GetAllIds()
}

//
//func LoadFlashMessages() []interface{} {
//	return a.FlashData
//}
//
//func CleanFlashMessages() string {
//	a.FlashData = a.FlashData[:0]
//	return ""
//}
