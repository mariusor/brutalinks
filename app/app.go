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

		"github.com/gorilla/sessions"
	"github.com/mariusor/littr.go/models"
	"golang.org/x/oauth2"
	"github.com/gin-gonic/gin"
)

const (
	sessionName   = "_s"
	templateDir   = "templates/"
	StatusUnknown = -1
)

var Db *sql.DB
var CurrentAccount = AnonymousAccount()
var SessionStore sessions.Store

const anonymous = "anonymous"
var defaultAccount = Account{Id: 0, Handle: anonymous, votes: make([]Vote, 0)}

const (
	Success = "success"
	Info = "info"
	Warning = "warning"
	Error = "error"
)

type Flash struct {
	Type string
	Msg  string
}

func AnonymousAccount() *Account {
	return &defaultAccount
}

func (a *Account) IsLogged() bool {
	return a != nil && (a.Handle != defaultAccount.Handle && a.CreatedAt != defaultAccount.CreatedAt)
}

type Littr struct {
	Host      string
	Port      int64
	Db        *sql.DB
	FlashData []interface{}
}

type errorModel struct {
	Status        int
	Title         string
	InvertedTheme bool
	Errors        []error
}

func GetSession(r *http.Request) *sessions.Session {
	s, err := SessionStore.Get(r, sessionName)
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

func RenderTemplate(r *http.Request, w http.ResponseWriter, name string, m interface{}) error {
	t, terr := LoadTemplates(templateDir, name, r)
	if terr != nil {
		log.Print(terr)
		AddFlashMessage(fmt.Sprint(terr), Error, r, w)
		return terr
	}
	terr = t.Execute(w, m)
	if terr != nil {
		log.Print(terr)
		AddFlashMessage(fmt.Sprint(terr), Error, r, w)
		return terr
	}
	sessions.Save(r, w)
	return nil
}

func LoadTemplates(base string, main string, r *http.Request) (*template.Template, error) {
	var terr error
	var t *template.Template
	t, terr = template.New(main).ParseFiles(base + main)
	if terr != nil {
		return nil, terr
	}

	t.Funcs(template.FuncMap{
		"isInverted":         IsInverted,
		"sluggify":           sluggify,
		"title":              func(t []byte) string { return string(t) },
		"getProviders":       getAuthProviders,
		"CurrentAccount":     func() *Account { return CurrentAccount },
		"LoadFlashMessages":  func () []interface{} {
			s := GetSession(r)
			return s.Flashes()
		},
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
	_, terr = t.New("submit.html").ParseFiles(base + "partials/new/submit.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("comments.html").ParseFiles(base + "partials/content/comments.html")
	if terr != nil {
		return nil, terr
		log.Print(terr)
	}
	_, terr = t.New("comment.html").ParseFiles(base + "partials/content/comment.html")
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
	_, terr = t.New("new-account.html").ParseFiles(base + "partials/register/new-account.html")
	if terr != nil {
		return nil, terr
		log.Print(terr)
	}
	_, terr = t.New("local-login.html").ParseFiles(base + "partials/login/local-login.html")
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

func AddVote(p models.Content, score int, userId int64) (bool, error) {
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
		rows, err := Db.Query(sel, userId, p2)
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
		res, err := Db.Exec(q, newWeight, p.Id, userId)
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
		res, err := Db.Exec(upd, v.Weight, newWeight, p.Id)
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

func (l *Littr) Run(m http.Handler, wait time.Duration) {
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
func HandleAdmin(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("done!!!"))
}

// handleMain serves /auth/{provider}/callback request
func (l *Littr) HandleCallback(c *gin.Context) {
	r := c.Request
	w := c.Writer

	vars := c.Params
	q := r.URL.Query()
	provider := vars.ByName("provider")
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

	s, err := SessionStore.Get(r, sessionName)
	if err != nil {
		log.Printf("ERROR %s", err)
	}

	s.Values["provider"] = provider
	s.Values["code"] = code
	s.Values["state"] = state
	s.AddFlash("Success")

	err = SessionStore.Save(r, w, s)
	if err != nil {
		log.Print(err)
	}
	http.Redirect(w, r, l.BaseUrl(), http.StatusFound)
}

// handleMain serves /auth/{provider}/callback request
func (l *Littr) HandleAuth(c *gin.Context) {
	r := c.Request
	w := c.Writer
	vars := c.Params
	provider := vars.ByName("provider")

	indexUrl := "/"
	if os.Getenv(strings.ToUpper(provider)+"_KEY") == "" {
		log.Printf("Provider %s has no credentials set", provider)
		http.Redirect(w, r, indexUrl, http.StatusPermanentRedirect)
		return
	}
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
		s, err := SessionStore.Get(r, sessionName)
		if err != nil {
			log.Printf("ERROR %s", err)
		}
		s.AddFlash("Missing oauth provider")
		http.Redirect(w, r, indexUrl, http.StatusPermanentRedirect)
	}
	http.Redirect(w, r, config.AuthCodeURL("state", oauth2.AccessTypeOnline), http.StatusFound)
}

func (l *Littr) LoggerMw(n http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.RequestURI)
		n.ServeHTTP(w, r)
	})
}

func IsInverted(r *http.Request) bool {
	cookies := r.Cookies()
	for _, c := range cookies {
		log.Printf("cookie %s:%s", c.Name, c.Value)
		if c.Name == "inverted" {
			return true
		}
	}
	return false
}

func (l *Littr) Sessions(c *gin.Context) {
	r := c.Request
	s := GetSession(r)
	l.FlashData = s.Flashes()
	if len(l.FlashData) > 0 {
		log.Printf("flashes %#v", l.FlashData)
		for _, err := range l.FlashData {
			log.Print(err)
		}
	}
	//for k, v := range s.Values {
	//	log.Printf("sess %s %#v", k, v)
	//}
	if s.Values[SessionUserKey] != nil {
		a := s.Values[SessionUserKey].(Account)
		CurrentAccount = &a
	} else {
		CurrentAccount = AnonymousAccount()
	}
}

func (l *Littr) AuthCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		s, err := SessionStore.Get(r, sessionName)
		if err != nil {
			log.Printf("ERROR %s", err)
		}
		log.Printf("%#v", s.Values)
		//l.SessionStore.Save(r, w, s)
	})
}

func AddFlashMessage(msg string, typ string, r *http.Request, w http.ResponseWriter) {
	s := GetSession(r)
	s.AddFlash(Flash{typ, msg})
}

//func LoadFlashMessages(r *http.Request) []interface{} {
//	s := GetSession(r)
//	return s.Flashes()
//}
//func CleanFlashMessages() string {
//	a.FlashData = a.FlashData[:0]
//	return ""
//}
