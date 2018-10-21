package frontend

import (
	"context"
	"encoding/gob"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"os"
	"strings"

	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"github.com/unrolled/render"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/go-chi/chi"
	"github.com/gorilla/sessions"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	mark "gitlab.com/golang-commonmark/markdown"
)

const (
	sessionName = "_s"
	templateDir = "templates/"
)

var sessionStore sessions.Store
var defaultAccount = models.Account{Handle: app.Anonymous, Hash: app.AnonymousHash}

var Logger log.FieldLogger
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

type Flash struct {
	Type flashType
	Msg  string
}

func html(i models.Item) template.HTML {
	return template.HTML(string(i.Data))
}

func markdown(data string) template.HTML {
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

func text(i models.Item) string {
	return string(i.Data)
}

const RendererCtxtKey = "__renderer"

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
				"CurrentAccount":    func() *models.Account { return CurrentAccount },
				"LoadFlashMessages": loadFlashMessages,
				"Mod10":             func(lvl uint8) float64 { return math.Mod(float64(lvl), float64(10)) },
				"ShowText":          func() bool { return ShowItemData },
				"HTML":              html,
				"Text":              text,
				"Markdown":          markdown,
				"PermaLink":         ItemPermaLink,
				"ParentLink":        parentLink,
				"OPLink":            opLink,
				"IsYay":             isYay,
				"IsNay":             isNay,
				"ScoreFmt":          scoreFmt,
				"NumberFmt":         func(i int64) string { return NumberFormat("%d", i) },
				"ScoreClass":        scoreClass,
				"YayLink":           yayLink,
				"NayLink":           nayLink,
				"PageLink":          pageLink,
				"version":           func() string { return app.Instance.Version },
				"Name":              func() template.HTML { return appName(app.Instance) },
				"Menu":              func() []template.HTML { return headerMenu(app.Instance, r) },
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

func init() {
	gob.Register(sessionAccount{})
	gob.Register(Flash{})

	s := sessions.NewCookieStore(app.Instance.SessionKeys[0], app.Instance.SessionKeys[1])
	s.Options.Domain = app.Instance.HostName
	s.Options.Path = "/"
	s.Options.HttpOnly = true
	s.Options.Secure = app.Instance.Secure

	sessionStore = s

	if Logger == nil {
		Logger = log.StandardLogger()
	}
}

func AnonymousAccount() *models.Account {
	return &defaultAccount
}

var FlashData = make([]Flash, 0)

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

func headerMenu(app app.Application, r *http.Request) []template.HTML {
	sections := []string{"self", "federated", "followed"}
	ret := make([]template.HTML, len(sections))
	for _, s := range sections {
		ret = append(ret, template.HTML(fmt.Sprintf(`<a class="%s icon" href="/%s">/%s</a>`, s, s, s)))
	}
	return ret
}

func appName(app app.Application) template.HTML {
	parts := strings.Split(app.Name(), " ")

	name := strings.Builder{}
	//name.WriteString("<small>")
	//name.WriteString(section)
	//name.WriteString("</small>")

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
		Logger.WithFields(log.Fields{
			"status": status,
			"url":    url,
		}).Error(err)
	}

	http.Redirect(w, r, url, status)
}

func RenderTemplate(r *http.Request, w http.ResponseWriter, name string, m interface{}) error {
	var err error
	if err = saveSession(w, r); err != nil {
		Logger.WithFields(log.Fields{
			"template": name,
			"model":    fmt.Sprintf("%#v", m),
		}).Error(err)
	}
	renderer, ok := r.Context().Value(RendererCtxtKey).(*render.Render)
	if !ok {
		err = errors.New("unable to load renderer")
		Logger.WithFields(log.Fields{}).Error(err)
		return err
	}
	if err = renderer.HTML(w, http.StatusOK, name, m); err != nil {
		new := errors.NewErr("failed to render template")
		Logger.WithFields(log.Fields{
			"template": name,
			"model":    fmt.Sprintf("%#v", m),
			"trace":    new.StackTrace(),
			"previous": err.Error(),
		}).Error(new)
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
		Logger.WithFields(log.Fields{}).Infof("ERROR %s", err)
	}

	s.Values["provider"] = provider
	s.Values["code"] = code
	s.Values["state"] = state
	addFlashMessage(Success, fmt.Sprintf("Login successful with %s", provider), r)

	err = sessionStore.Save(r, w, s)
	if err != nil {
		Logger.WithFields(log.Fields{}).Info(err)
	}
	Redirect(w, r, "/", http.StatusFound)
}

// handleMain serves /auth/{provider}/callback request
func HandleAuth(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	indexUrl := "/"
	if os.Getenv(strings.ToUpper(provider)+"_KEY") == "" {
		Logger.WithFields(log.Fields{}).Infof("Provider %s has no credentials set", provider)
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
			Logger.WithFields(log.Fields{}).Infof("ERROR %s", err)
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

func loadCurrentAccount(s *sessions.Session) models.Account {
	// load the current account from the session or setting it to anonymous
	if raw, ok := s.Values[SessionUserKey]; ok {
		if a, ok := raw.(sessionAccount); ok {
			if acc, err := db.Config.LoadAccount(models.LoadAccountsFilter{Handle: []string{a.Handle}}); err == nil {
				Logger.WithFields(log.Fields{
					"handle": acc.Handle,
					"hash":   acc.Hash,
				}).Debugf("loaded account from session")
				return acc
			} else {
				if err != nil {
					Logger.WithFields(log.Fields{
						"handle": a.Handle,
						"hash":   a.Hash,
					}).Warn(err)
				}
			}
		}
	}
	return defaultAccount
}

func loadSessionFlashMessages(s *sessions.Session) {
	FlashData = FlashData[:0]
	flashes := s.Flashes()
	// setting the local FlashData value
	for _, int := range flashes {
		if int == nil {
			continue
		}
		f, ok := int.(Flash)
		if !ok {
			Logger.WithFields(log.Fields{}).Error(errors.NewErr("unable to read flash struct from %T %#v", int, int))
		}
		FlashData = append(FlashData, f)
	}
}

func LoadSession(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		acc := defaultAccount
		if s, err := sessionStore.Get(r, sessionName); err != nil {
			Logger.WithFields(log.Fields{}).Error(err)
		} else {
			loadSessionFlashMessages(s)
			acc = loadCurrentAccount(s)
		}
		ctx := context.WithValue(r.Context(), models.AccountCtxtKey, &acc)

		CurrentAccount = &acc
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func addFlashMessage(typ flashType, msg string, r *http.Request) {
	//s, _ := sessionStore.Get(r, sessionName)
	n := Flash{typ, msg}

	exists := false
	for _, f := range FlashData {
		if f == n {
			exists = true
			break
		}
	}
	if !exists {
		//s.AddFlash(n)
	}
}

func loadFlashMessages() []Flash {
	f := FlashData
	FlashData = nil
	return f
}
