package frontend

import (
	"context"
	"encoding/gob"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"github.com/unrolled/render"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/go-chi/chi"
	"github.com/gorilla/sessions"
	log "github.com/inconshreveable/log15"
	"github.com/juju/errors"
	"golang.org/x/oauth2"

	mark "gitlab.com/golang-commonmark/markdown"
)

const (
	sessionName = "_s"
	templateDir = "templates/"
)

var sessionStore sessions.Store
var defaultAccount = app.Account{Handle: app.Anonymous, Hash: app.AnonymousHash}

var Logger log.Logger

//var Renderer *render.Render
var CurrentAccount = &defaultAccount
var ShowItemData = false

type flashType string

const (
	Success flashType = "success"
	Info    flashType = "info"
	Warning flashType = "warning"
	Error   flashType = "error"
)

type flash struct {
	Type flashType
	Msg  string
}

func html(i app.Item) template.HTML {
	return template.HTML(string(i.Data))
}

func Markdown(data string) template.HTML {
	md := mark.New(
		mark.HTML(true),
		mark.Tables(true),
		mark.Linkify(false),
		mark.Breaks(false),
		mark.Typographer(true),
		mark.XHTMLOutput(false),
	)

	h := md.RenderToString([]byte(data))
	return template.HTML(h)
}

func text(i app.Item) string {
	return string(i.Data)
}

func RelTimeLabel(old time.Time) string {
	//return humanize.RelTime(old, time.Now(), "ago", "in the future")
	td := time.Now().Sub(old)
	pluralize := func(d float64, unit string) string {
		if math.Round(d) != 1 {
			if unit == "century" {
				unit = "centurie"
			}
			return unit + "s"
		}
		return unit
	}
	val := 0.0
	unit := ""
	when := "ago"

	hours := math.Abs(td.Hours())
	minutes := math.Abs(td.Minutes())
	seconds := math.Abs(td.Seconds())

	if td.Seconds() < 0 {
		// we're in the future
		when = "in the future"
	}
	if seconds < 30 {
		return "now"
	}
	if hours < 1 {
		if minutes < 1 {
			val = math.Mod(seconds, 60)
			unit = "second"
		} else {
			val = math.Mod(minutes, 60)
			unit = "minute"
		}
	} else if hours < 24 {
		val = hours
		unit = "hour"
	} else if hours < 168 {
		val = hours / 24
		unit = "day"
	} else if hours < 672 {
		val = hours / 168
		unit = "week"
	} else if hours < 8760 {
		val = hours / 672
		unit = "month"
	} else if hours < 87600 {
		val = hours / 8760
		unit = "year"
	} else if hours < 876000 {
		val = hours / 87600
		unit = "decade"
	} else {
		val = hours / 876000
		unit = "century"
	}
	return fmt.Sprintf("%.0f %s %s", val, pluralize(val, unit), when)
}

const RendererCtxtKey = "__renderer"

func Init(app *app.Application) error {
	// frontend
	gob.Register(sessionAccount{})
	gob.Register(flash{})
	return InitSessionStore(app)
}

// InitSessionStore initializes the session store if we have encryption key settings in the env variables
func InitSessionStore(app *app.Application) error {
	if len(app.SessionKeys) > 0 {
		s := sessions.NewCookieStore(app.SessionKeys...)
		s.Options.Domain = app.HostName
		s.Options.Path = "/"
		s.Options.HttpOnly = true
		s.Options.Secure = app.Secure

		sessionStore = s
	} else {
		err := errors.New("no session encryption configuration, unable to use sessions")
		Logger.Warn(err.Error())
		app.Config.SessionsEnabled = false
		return err
	}
	return nil
}

func Renderer(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		renderer := render.New(render.Options{
			Directory:  templateDir,
			Asset:      nil,
			AssetNames: nil,
			Layout:     "layout",
			Extensions: []string{".html"},
			Funcs: []template.FuncMap{{
				//"urlParam":          func(s string) string { return chi.URLParam(r, s) },
				//"get":               func(s string) string { return r.URL.Query().Get(s) },
				"isInverted":        isInverted,
				"sluggify":          sluggify,
				"title":             func(t []byte) string { return string(t) },
				"getProviders":      getAuthProviders,
				"CurrentAccount":    func() *app.Account { return CurrentAccount },
				"LoadFlashMessages": loadFlashMessages,
				"Mod10":             func(lvl uint8) float64 { return math.Mod(float64(lvl), float64(10)) },
				"ShowText":          func() bool { return ShowItemData },
				"HTML":              html,
				"Text":              text,
				"Markdown":          Markdown,
				"PermaLink":         ItemPermaLink,
				"ParentLink":        parentLink,
				"OPLink":            opLink,
				"IsYay":             isYay,
				"IsNay":             isNay,
				"ScoreFmt":          scoreFmt,
				"NumberFmt":         func(i int64) string { return NumberFormat("%d", i) },
				"TimeFmt":           RelTimeLabel,
				//"ScoreFmt":          func(i int64) string { return humanize.FormatInteger("#\u202F###", int(i)) },
				//"NumberFmt":         func(i int64) string { return humanize.FormatInteger("#\u202F###", int(i)) },
				"ScoreClass": scoreClass,
				"YayLink":    yayLink,
				"NayLink":    nayLink,
				"PageLink":   pageLink,
				"App":        func() app.Application { return app.Instance },
				"Name":       appName,
				"Menu":       func() []template.HTML { return headerMenu(r) },
			}},
			Delims:         render.Delims{Left: "{{", Right: "}}"},
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
		ctx := context.WithValue(r.Context(), RendererCtxtKey, renderer)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func AnonymousAccount() *app.Account {
	return &defaultAccount
}

var flashData = make([]flash, 0)

type errorModel struct {
	Status        int
	Title         string
	InvertedTheme bool
	Errors        []error
}

const (
	ScoreMaxK = 1000.0
	ScoreMaxM = 1000000.0
	ScoreMaxB = 1000000000.0
)

func loadScoreFormat(s int64) (string, string) {
	score := 0.0
	units := ""
	base := float64(s)
	d := math.Ceil(math.Log10(math.Abs(base)))
	dK := math.Ceil(math.Log10(math.Abs(ScoreMaxK)))
	dM := math.Ceil(math.Log10(math.Abs(ScoreMaxM)))
	dB := math.Ceil(math.Log10(math.Abs(ScoreMaxB)))
	if d < dK {
		score = math.Ceil(base)
		return NumberFormat("%d", int(score)), ""
	} else if d < dM {
		score = base / ScoreMaxK
		units = "K"
	} else if d < dB {
		score = base / ScoreMaxM
		units = "M"
	} else if d < dB+2 {
		score = base / ScoreMaxB
		units = "B"
	} else {
		sign := ""
		if base < 0 {
			sign = "&ndash;"
		}
		return fmt.Sprintf("%s%s", sign, "∞"), "inf"
	}

	return NumberFormat("%3.1f", score), units
}

func NumberFormat(fmtVerb string, el ...interface{}) string {
	return message.NewPrinter(language.English).Sprintf(fmtVerb, el...)
}

func scoreClass(s int64) string {
	_, class := loadScoreFormat(s)
	if class == "" {
		class = "H"
	}
	return class
}

func scoreFmt(s int64) string {
	score, units := loadScoreFormat(s)
	if units == "inf" {
		units = ""
	}
	return fmt.Sprintf("%s%s", score, units)
}

func headerMenu(r *http.Request) []template.HTML {
	sections := []string{"self", "federated", "followed"}
	ret := make([]template.HTML, 0)
	for _, s := range sections {
		if path.Base(r.URL.Path) == s {
			ret = append(ret, template.HTML(fmt.Sprintf(`<span class="%s icon" href="/%s">/%s</span>`, s, s, s)))
		} else {
			ret = append(ret, template.HTML(fmt.Sprintf(`<a class="%s icon" href="/%s">/%s</a>`, s, s, s)))
		}
	}

	return ret
}

func appName(app app.Application) template.HTML {
	parts := strings.Split(app.Name(), " ")

	name := strings.Builder{}

	name.WriteString("<strong>")
	name.WriteString(parts[0])
	name.WriteString("</strong>")

	for i, p := range parts {
		if i == 0 {
			continue
		}
		name.WriteString(" <small>")
		name.WriteString(p)
		name.WriteString("</small>")
	}

	return template.HTML(name.String())
}

func saveSession(w http.ResponseWriter, r *http.Request) error {
	var err error
	var s *sessions.Session
	if sessionStore == nil {
		err := errors.New("missing session store, unable to save session")
		Logger.Warn(err.Error())
		return err
	}
	if s, err = sessionStore.Get(r, sessionName); err != nil {
		return errors.Errorf("failed to load session before redirect: %s", err)
	}
	if err := sessionStore.Save(r, w, s); err != nil {
		return errors.Errorf("failed to save session before redirect: %s", err)
	}
	return nil
}

func Redirect(w http.ResponseWriter, r *http.Request, url string, status int) {
	if err := saveSession(w, r); err != nil {
		Logger.Error(err.Error(), log.Ctx{
			"status": status,
			"url":    url,
		})
	}

	http.Redirect(w, r, url, status)
}

func RenderTemplate(r *http.Request, w http.ResponseWriter, name string, m interface{}) error {
	var err error
	if err = saveSession(w, r); err != nil {
		Logger.Error(err.Error(), log.Ctx{
			"template": name,
			"model":    fmt.Sprintf("%#v", m),
		})
	}
	renderer, ok := r.Context().Value(RendererCtxtKey).(*render.Render)
	if !ok {
		err = errors.New("unable to load renderer")
		Logger.Error(err.Error())
		return err
	}
	if err = renderer.HTML(w, http.StatusOK, name, m); err != nil {
		new := errors.NewErr("failed to render template")
		Logger.Error(new.Error(), log.Ctx{
			"template": name,
			"model":    fmt.Sprintf("%#v", m),
			"trace":    new.StackTrace(),
			"previous": err.Error(),
		})
		renderer.HTML(w, http.StatusInternalServerError, "error", new)
	}
	return err
}

// handleAdmin serves /admin request
func HandleAdmin(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("done!!!"))
}

// handleMain serves /auth/{provider}/callback request
func HandleCallback(w http.ResponseWriter, r *http.Request) {
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

	s, err := sessionStore.Get(r, sessionName)
	if err != nil {
		Logger.Info(err.Error())
	}

	s.Values["provider"] = provider
	s.Values["code"] = code
	s.Values["state"] = state
	//addFlashMessage(Success, fmt.Sprintf("Login successful with %s", provider), r)

	err = sessionStore.Save(r, w, s)
	if err != nil {
		Logger.Info(err.Error())
	}
	Redirect(w, r, "/", http.StatusFound)
}

// handleMain serves /auth/{provider}/callback request
func HandleAuth(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	indexUrl := "/"
	if os.Getenv(strings.ToUpper(provider)+"_KEY") == "" {
		Logger.Info("Provider has no credentials set", log.Ctx{"provider": provider})
		Redirect(w, r, indexUrl, http.StatusPermanentRedirect)
		return
	}
	url := fmt.Sprintf("%s/auth/%s/callback", "", provider)

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
		s, err := sessionStore.Get(r, sessionName)
		if err != nil {
			Logger.Info(err.Error())
		}
		s.AddFlash("Missing oauth provider")
		Redirect(w, r, indexUrl, http.StatusPermanentRedirect)
	}
	Redirect(w, r, config.AuthCodeURL("state", oauth2.AccessTypeOnline), http.StatusFound)
}

func isInverted(r *http.Request) bool {
	cookies := r.Cookies()
	for _, c := range cookies {
		if c.Name == "inverted" {
			return true
		}
	}
	return false
}

func loadCurrentAccount(s *sessions.Session) app.Account {
	// load the current account from the session or setting it to anonymous
	if raw, ok := s.Values[SessionUserKey]; ok {
		if a, ok := raw.(sessionAccount); ok {
			if acc, err := db.Config.LoadAccount(app.LoadAccountsFilter{Handle: []string{a.Handle}}); err == nil {
				Logger.Debug("loaded account from session", log.Ctx{
					"handle": acc.Handle,
					"hash":   acc.Hash,
				})
				return acc
			} else {
				if err != nil {
					Logger.Warn(err.Error(), log.Ctx{
						"handle": a.Handle,
						"hash":   a.Hash,
					})
				}
			}
		}
	}
	return defaultAccount
}

func loadSessionFlashMessages(s *sessions.Session) {
	flashData = flashData[:0]
	flashes := s.Flashes()
	// setting the local flashData value
	for _, int := range flashes {
		if int == nil {
			continue
		}
		f, ok := int.(flash)
		if !ok {
			Logger.Error("unable to read flash struct", log.Ctx{
				"type": fmt.Sprintf("%T", int),
				"val": fmt.Sprintf("%#v", int),
			})
		}
		flashData = append(flashData, f)
	}
}

func LoadSession(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		acc := defaultAccount
		if !app.Instance.Config.SessionsEnabled {
			err := errors.New("session store disabled")
			Logger.Warn(err.Error())
			next.ServeHTTP(w, r)
			return
		}
		if sessionStore == nil {
			err := errors.New("missing session store, unable to load session")
			Logger.Warn(err.Error())
			next.ServeHTTP(w, r)
			return
		}
		if s, err := sessionStore.Get(r, sessionName); err != nil {
			Logger.Error(err.Error())
		} else {
			loadSessionFlashMessages(s)
			acc = loadCurrentAccount(s)
		}
		ctx := context.WithValue(r.Context(), app.AccountCtxtKey, &acc)

		CurrentAccount = &acc
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func addFlashMessage(typ flashType, msg string, r *http.Request) {
	//s, _ := sessionStore.Get(r, sessionName)
	n := flash{typ, msg}

	exists := false
	for _, f := range flashData {
		if f == n {
			exists = true
			break
		}
	}
	if !exists {
		//s.AddFlash(n)
	}
}

func loadFlashMessages() []flash {
	f := flashData
	flashData = nil
	return f
}

func NeedsSessions(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if !app.Instance.Config.SessionsEnabled {
			HandleError(w, r, http.StatusNotFound, errors.New("sessions are disabled"))
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
