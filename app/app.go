package app

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/gorilla/sessions"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
	"github.com/unrolled/render"
	"golang.org/x/oauth2"
	)

const (
	sessionName   = "_s"
	templateDir   = "templates/"
	StatusUnknown = -1
)

var Db *sql.DB
var SessionStore sessions.Store

var CurrentAccount *Account
var Renderer *render.Render

const anonymous = "anonymous"

var defaultAccount = Account{Id: 0, Handle: anonymous, votes: make([]Vote, 0)}

const (
	Success = "success"
	Info    = "info"
	Warning = "warning"
	Error   = "error"
)

type Flash struct {
	Type string
	Msg  string
}

func init() {
	Renderer = render.New(render.Options{
		Directory:  templateDir,
		Asset:      nil,
		AssetNames: nil,
		Layout:     "layout",
		Extensions: []string{".html"},
		Funcs: []template.FuncMap{{
			"isInverted":     IsInverted,
			"sluggify":       sluggify,
			"title":          func(t []byte) string { return string(t) },
			"getProviders":   getAuthProviders,
			"CurrentAccount": func() *Account { return CurrentAccount },
			"LoadFlashMessages": func() []interface{} {
				//s := GetSession(r)
				//return s.Flashes()
				return []interface{}{}
			},
			"mod":                func(lvl int) float64 { return math.Mod(float64(lvl), float64(10)) },
			"CleanFlashMessages": func() string { return "" }, //CleanFlashMessages,
		}},
		Delims:         render.Delims{"{{", "}}"},
		Charset:        "UTF-8",
		DisableCharset: false,
		//IndentJSON: false,
		//IndentXML: false,
		//PrefixJSON: []byte(""),
		//PrefixXML: []byte(""),
		BinaryContentType: "application/octet-stream",
		HTMLContentType:   "text/html",
		//JSONContentType: "application/json",
		//JSONPContentType: "application/javascript",
		//TextContentType: "text/plain",
		//XMLContentType: "application/xhtml+xml",
		IsDevelopment: true,
		//UnEscapeHTML: false,
		//StreamingJSON: false,
		//RequirePartials: false,
		DisableHTTPErrorRendering: false,
	})

	if CurrentAccount == nil {
		CurrentAccount = AnonymousAccount()
	}
}

func AnonymousAccount() *Account {
	return &defaultAccount
}

func (a *Account) IsLogged() bool {
	return a != nil && (a.Handle != defaultAccount.Handle && a.CreatedAt != defaultAccount.CreatedAt)
}

type EnvType string

const DEV EnvType = "development"
const PROD EnvType = "production"
const QA EnvType = "qa"

var validEnvTypes = []EnvType{
	DEV,
	PROD,
}

func ValidEnv(s EnvType) bool {
	for _, k := range validEnvTypes {
		if k == s {
			return true
		}
	}
	return false
}

type Littr struct {
	Env         EnvType
	HostName    string
	Port        int64
	Listen      string
	Db          *sql.DB
	SessionKeys [2][]byte
	FlashData   []interface{}
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
		log.WithField("context", "Session").Error(err)
	}
	return s
}

func (l *Littr) listen() string {
	if len(l.Listen) > 0 {
		return l.Listen
	}
	var port string
	if l.Port != 0 {
		port = fmt.Sprintf(":%d", l.Port)
	}
	return fmt.Sprintf("%s%s", l.HostName, port)
}

func (l *Littr) BaseUrl() string {
	return fmt.Sprintf("http://%s", l.HostName)
}

func RenderTemplate(r *http.Request, w http.ResponseWriter, name string, m interface{}) error {
	err := Renderer.HTML(w, http.StatusOK, name, m)
	if err != nil {
		Renderer.HTML(w, http.StatusInternalServerError, "error", err)
		return err
	}
	return sessions.Save(r, w)
}

// AddVote adds a vote to the p content item
//   const {
//      add_vote = "add_vote"
//      delete = "delete"
//   }
//   type queue_message struct {
//       type    string
//       payload json.RawMessage
//   }
// Ideally this should be done asynchronously pushing an add_vote message to our
// messaging queue. Details of this queue to be established (strongest possibility is Redis PubSub)
// The cli/votes/main.go script would be responsible with waiting on the queue for these messages
// and updating the new score and all models dependent on it.
//   content_items and accounts tables, corresponding ES documents, etc
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
			return false, errors.Errorf("scoring %d failed on item %q", newWeight, p.Hash())
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
			return false, errors.Errorf("content hash %q not found", p.Hash())
		}
		if rows, _ := res.RowsAffected(); rows > 1 {
			return false, errors.Errorf("content hash %q collision", p.Hash())
		}
		log.Printf("updated content_items with %d", newWeight)
	}

	return true, nil
}

func (l *Littr) Run(m http.Handler, wait time.Duration) {
	log.Infof("starting debug level %q", log.GetLevel().String())
	log.Infof("listening on %s", l.listen())

	srv := &http.Server{
		Addr: l.listen(),
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      m,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Error(err)
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
	log.Infof("shutting down")
	os.Exit(0)
}

// handleAdmin serves /admin request
func HandleAdmin(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("done!!!"))
}

// handleMain serves /auth/{provider}/callback request
func (l *Littr) HandleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	provider := chi.URLParam(r, "provider")
	providerErr := q["error"]
	if providerErr != nil {
		errDescriptions := q["error_description"]
		var errs = make([]error, 1)
		errs[0] = errors.Errorf("Error for provider %q:\n", provider)
		for _, errDesc := range errDescriptions {
			errs = append(errs, errors.Errorf(errDesc))
		}
		HandleError(w, r, http.StatusForbidden, errs...)
		return
	}
	code := q["code"]
	state := q["state"]
	if code == nil {
		HandleError(w, r, http.StatusForbidden, errors.Errorf("%s error: Empty authentication token", provider))
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
func (l *Littr) HandleAuth(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

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

func IsInverted(r *http.Request) bool {
	cookies := r.Cookies()
	for _, c := range cookies {
		if c.Name == "inverted" {
			return true
		}
	}
	return false
}

func (l *Littr) Sessions(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		s := GetSession(r)
		l.FlashData = s.Flashes()
		if len(l.FlashData) > 0 {
			log.Debugf("flashes %#v", l.FlashData)
			for _, errMsg := range l.FlashData {
				log.Error(errMsg)
			}
		}
		if s.Values[SessionUserKey] != nil {
			a := s.Values[SessionUserKey].(Account)
			CurrentAccount = &a
		} else {
			CurrentAccount = AnonymousAccount()
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func (l *Littr) AuthCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		s, err := SessionStore.Get(r, sessionName)
		if err != nil {
			log.Error(err)
		}
		log.Debugf("%#v", s.Values)
		//l.SessionStore.Save(r, w, s)
	})
}

func AddFlashMessage(msg string, typ string, r *http.Request, w http.ResponseWriter) {
	s := GetSession(r)
	s.AddFlash(Flash{typ, msg})
}

func InvertedMw(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			IsInverted(r)
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

//func LoadFlashMessages(r *http.Request) []interface{} {
//	s := GetSession(r)
//	return s.Flashes()
//}
//func CleanFlashMessages() string {
//	a.FlashData = a.FlashData[:0]
//	return ""
//}
