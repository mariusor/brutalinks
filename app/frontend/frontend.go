package frontend

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"github.com/unrolled/render"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/go-chi/chi"
	"github.com/gorilla/sessions"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app/log"
	"golang.org/x/oauth2"
)

const (
	sessionName = "_s"
	templateDir = "templates/"
	assetsDir   = "assets/"
)

type handler struct {
	session  sessions.Store
	account  app.Account
	logger   log.Logger
}

var defaultAccount = app.Account{Handle: app.Anonymous, Hash: app.AnonymousHash}

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

func html(data string) template.HTML {
	return template.HTML(data)
}

func text(data string) string {
	return string(data)
}

func icon(icon string, c ...string) template.HTML {
	cls := make([]string, 0)
	cls = append(cls, c...)

	buf := fmt.Sprintf(`<svg class="icon icon-%s %s">
	<use xlink:href="#icon-%s" />
</svg>`, icon, strings.Join(cls, " "), icon)

	return template.HTML(buf)
}

func asset(p string) []byte {
	file := filepath.Clean(p)
	fullPath := filepath.Join(assetsDir, file)

	f, err := os.Open(fullPath)
	if err != nil {
		return []byte{0}
	}
	defer f.Close()

	r := bufio.NewReader(f)
	b := bytes.Buffer{}
	_, err = r.WriteTo(&b)
	if err != nil {
		return []byte{0}
	}

	return b.Bytes()
}

func isoTimeFmt(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.000-07:00")
}

func relTimeFmt(old time.Time) string {
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

type Config struct {
	Version     string
	BaseURL     string
	HostName    string
	Secure      bool
	SessionKeys [][]byte
	Logger      log.Logger
}

func Init(c Config) (handler, error) {
	// frontend
	gob.Register(sessionAccount{})
	gob.Register(flash{})

	var err error

	h := handler{
		account: defaultAccount,
	}
	if c.Logger != nil {
		h.logger = c.Logger
	}
	h.session, err = InitSessionStore(c)

	return h, err
}

// InitSessionStore initializes the session store if we have encryption key settings in the env variables
func InitSessionStore(c Config) (sessions.Store, error) {
	var s sessions.Store
	if len(c.SessionKeys) > 0 {
		ss := sessions.NewCookieStore(c.SessionKeys...)
		//s.Options.Domain = c.HostName
		ss.Options.Path = "/"
		ss.Options.HttpOnly = true
		ss.Options.Secure = c.Secure
		s = ss
	} else {
		err := errors.New("no session encryption configuration, unable to use sessions")
		if c.Logger != nil {
			c.Logger.Warn(err.Error())
		}
		//app.Config.SessionsEnabled = false
		return nil, err
	}
	return s, nil
}

func AnonymousAccount() app.Account {
	return defaultAccount
}

var flashData = make([]flash, 0)

type errorModel struct {
	Status        int
	Title         string
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
	dK := 4.0  // math.Ceil(math.Log10(math.Abs(ScoreMaxK))) + 1
	dM := 7.0  // math.Ceil(math.Log10(math.Abs(ScoreMaxM))) + 1
	dB := 10.0 // math.Ceil(math.Log10(math.Abs(ScoreMaxB))) + 1
	if d < dK {
		score = math.Ceil(base)
		return numberFormat("%d", int(score)), ""
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

	return numberFormat("%3.1f", score), units
}

func numberFormat(fmtVerb string, el ...interface{}) string {
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

type headerEl struct {
	IsCurrent bool
	Icon      string
	Name      string
	URL       string
}

func headerMenu(r *http.Request) []headerEl {
	sections := []string{"self", "federated", "followed"}
	ret := make([]headerEl, 0)
	for _, s := range sections {
		el := headerEl{
			Name: s,
			URL:  fmt.Sprintf("%s/%s", app.Instance.BaseURL, s),
		}
		if path.Base(r.URL.Path) == s {
			el.IsCurrent = true
		}
		switch s {
		case "self":
			el.Icon = "home"
		case "federated":
			el.Icon = "activitypub"
		case "followed":
			el.Icon = "star"
		}
		ret = append(ret, el)
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

func (h handler) saveSession(w http.ResponseWriter, r *http.Request) error {
	var err error
	var s *sessions.Session
	if h.session == nil {
		err := errors.New("missing session store, unable to save session")
		return err
	}
	if s, err = h.session.Get(r, sessionName); err != nil {
		return errors.Errorf("failed to load session before redirect: %s", err)
	}
	if err := h.session.Save(r, w, s); err != nil {
		return errors.Errorf("failed to save session before redirect: %s", err)
	}
	return nil
}

func (h handler) Redirect(w http.ResponseWriter, r *http.Request, url string, status int) {
	if err := h.saveSession(w, r); err != nil {
		h.logger.WithContext(log.Ctx{
			"status": status,
			"url":    url,
		}).Error(err.Error())
	}

	http.Redirect(w, r, url, status)
}

func sameHash(s1 string, s2 string) bool {
	return path.Base(s1) == path.Base(s2)
}

func (h handler) RenderTemplate(r *http.Request, w http.ResponseWriter, name string, m interface{}) error {
	var err error
	if err = h.saveSession(w, r); err != nil {
		h.logger.WithContext(log.Ctx{
			"template": name,
			"model":    fmt.Sprintf("%#v", m),
		}).Error(err.Error())
	}
	ren := render.New(render.Options{
		Directory:  templateDir,
		Layout:     "layout",
		Extensions: []string{".html"},
		Funcs: []template.FuncMap{{
			//"urlParam":          func(s string) string { return chi.URLParam(r, s) },
			//"get":               func(s string) string { return r.URL.Query().Get(s) },
			"isInverted":        func () bool { return isInverted(r) },
			"sluggify":          sluggify,
			"title":             func(t []byte) string { return string(t) },
			"getProviders":      getAuthProviders,
			"CurrentAccount":    func() app.Account { return h.account },
			"LoadFlashMessages": loadFlashMessages,
			"Mod10":             func(lvl uint8) float64 { return math.Mod(float64(lvl), float64(10)) },
			"ShowText":          func() bool {
				mm, ok := m.(itemListingModel)
				return !ok || !mm.HideText
			},
			"HTML":              html,
			"Text":              text,
			"Markdown":          app.Markdown,
			"AccountPermaLink":  AccountPermaLink,
			"PermaLink":         ItemPermaLink,
			"ParentLink":        parentLink,
			"OPLink":            opLink,
			"IsYay":             isYay,
			"IsNay":             isNay,
			"ScoreFmt":          scoreFmt,
			"NumberFmt":         func(i int64) string { return numberFormat("%d", i) },
			"TimeFmt":           relTimeFmt,
			"ISOTimeFmt":        isoTimeFmt,
			"ScoreClass":        scoreClass,
			"YayLink":           yayLink,
			"NayLink":           nayLink,
			"PageLink":          pageLink,
			"App":               func() app.Application { return app.Instance },
			"Name":              appName,
			"Menu":              func() []headerEl { return headerMenu(r) },
			"icon":              icon,
			"asset":             func(p string) template.HTML { return template.HTML(asset(p)) },
			"req":               func() *http.Request { return r },
			"sameHash":          sameHash,
			//"ScoreFmt":          func(i int64) string { return humanize.FormatInteger("#\u202F###", int(i)) },
			//"NumberFmt":         func(i int64) string { return humanize.FormatInteger("#\u202F###", int(i)) },
		}},
		Delims:                    render.Delims{Left: "{{", Right: "}}"},
		Charset:                   "UTF-8",
		DisableCharset:            false,
		BinaryContentType:         "application/octet-stream",
		HTMLContentType:           "text/html",
		IsDevelopment:             true,
		DisableHTTPErrorRendering: false,
	})

	if app.Instance.Config.Env == app.DEV {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if err = ren.HTML(w, http.StatusOK, name, m); err != nil {
		new := errors.NewErr("failed to render template")
		h.logger.WithContext(log.Ctx{
			"template": name,
			"model":    fmt.Sprintf("%T", m),
			"trace":    new.StackTrace(),
			"previous": err.Error(),
		}).Error(new.Error())
		ren.HTML(w, http.StatusInternalServerError, "error", new)
	}
	return err
}

// handleAdmin serves /admin request
func (h handler) HandleAdmin(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("done!!!"))
}

// handleMain serves /auth/{provider}/callback request
func (h handler) HandleCallback(w http.ResponseWriter, r *http.Request) {
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
		h.HandleError(w, r, errs...)
		return
	}
	code := q["code"]
	state := q["state"]
	if code == nil {
		h.HandleError(w, r, errors.Forbiddenf("%s error: Empty authentication token", provider))
		return
	}

	s, err := h.session.Get(r, sessionName)
	if err != nil {
		h.logger.Info(err.Error())
	}

	s.Values["provider"] = provider
	s.Values["code"] = code
	s.Values["state"] = state
	//addFlashMessage(Success, fmt.Sprintf("Login successful with %s", provider), r)

	err = h.session.Save(r, w, s)
	if err != nil {
		h.logger.Info(err.Error())
	}
	h.Redirect(w, r, "/", http.StatusFound)
}

// handleMain serves /auth/{provider}/callback request
func (h handler) HandleAuth(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	indexUrl := "/"
	if os.Getenv(strings.ToUpper(provider)+"_KEY") == "" {
		h.logger.WithContext(log.Ctx{
			"provider": provider,
		}).Info("Provider has no credentials set")
		h.Redirect(w, r, indexUrl, http.StatusPermanentRedirect)
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
		s, err := h.session.Get(r, sessionName)
		if err != nil {
			h.logger.Info(err.Error())
		}
		s.AddFlash("Missing oauth provider")
		h.Redirect(w, r, indexUrl, http.StatusPermanentRedirect)
	}
	h.Redirect(w, r, config.AuthCodeURL("state", oauth2.AccessTypeOnline), http.StatusFound)
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

func loadCurrentAccount(s *sessions.Session, l log.Logger) app.Account {
	// load the current account from the session or setting it to anonymous
	if raw, ok := s.Values[SessionUserKey]; ok {
		if a, ok := raw.(sessionAccount); ok {
			if acc, err := db.Config.LoadAccount(app.LoadAccountsFilter{Handle: []string{a.Handle}}); err == nil {
				l.WithContext(log.Ctx{
					"handle": acc.Handle,
					"hash":   acc.Hash.String(),
				}).Debug("loaded account from session")
				return acc
			} else {
				if err != nil {
					l.WithContext(log.Ctx{
						"handle": a.Handle,
						"hash":   string(a.Hash),
					}).Warn(err.Error())
				}
			}
		}
	}
	return defaultAccount
}

func loadSessionFlashMessages(s *sessions.Session, l log.Logger) {
	flashData = flashData[:0]
	flashes := s.Flashes()
	// setting the local flashData value
	for _, int := range flashes {
		if int == nil {
			continue
		}
		f, ok := int.(flash)
		if !ok {
			l.WithContext(log.Ctx{
				"type": fmt.Sprintf("%T", int),
				"val":  fmt.Sprintf("%#v", int),
			}).Error("unable to read flash struct")
		}
		flashData = append(flashData, f)
	}
}

func (h *handler) LoadSession(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if !app.Instance.Config.SessionsEnabled {
			err := errors.New("session store disabled")
			h.logger.Warn(err.Error())
			next.ServeHTTP(w, r)
			return
		}
		if h.session == nil {
			err := errors.New("missing session store, unable to load session")
			h.logger.Warn(err.Error())
			next.ServeHTTP(w, r)
			return
		}
		if s, err := h.session.Get(r, sessionName); err != nil {
			h.logger.Error(err.Error())
		} else {
			h.account = loadCurrentAccount(s, h.logger)
			loadSessionFlashMessages(s, h.logger)
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func addFlashMessage(typ flashType, msg string, r *http.Request) {
	//s, _ := h.session.Get(r, sessionName)
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

func (h handler) NeedsSessions(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if !app.Instance.Config.SessionsEnabled {
			h.HandleError(w, r, errors.NotFoundf("sessions are disabled"))
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// HandleAbout serves /about request
// It's something Mastodon compatible servers should show
func (h *handler) HandleAbout(w http.ResponseWriter, r *http.Request) {
	m := aboutModel{Title: "About"}
	f, err := db.Config.LoadInfo()
	if err != nil {
		h.HandleError(w, r, errors.NewNotValid(err, "oops!"))
		return
	}
	m.Desc.Description = f.Description

	h.RenderTemplate(r, w, "about", m)
}

func httpErrorResponse(e error) int {
	if errors.IsBadRequest(e) {
		return http.StatusBadRequest
	}
	if errors.IsForbidden(e) {
		return http.StatusForbidden
	}
	if errors.IsNotSupported(e) {
		return http.StatusHTTPVersionNotSupported
	}
	if errors.IsMethodNotAllowed(e) {
		return http.StatusMethodNotAllowed
	}
	if errors.IsNotFound(e) {
		return http.StatusNotFound
	}
	if errors.IsNotImplemented(e) {
		return http.StatusNotImplemented
	}
	if errors.IsUnauthorized(e) {
		return http.StatusUnauthorized
	}
	if errors.IsTimeout(e) {
		return http.StatusGatewayTimeout
	}
	if errors.IsNotValid(e) {
		return http.StatusInternalServerError
	}
	return http.StatusInternalServerError
}

// HandleError serves failed requests
func (h *handler) HandleError(w http.ResponseWriter, r *http.Request, errs ...error) {
	d := errorModel{
		Errors:        errs,
	}

	status := http.StatusInternalServerError
	for _, err := range errs {
		if err != nil {
			h.logger.WithContext(log.Ctx{
				"trace": errors.ErrorStack(err),
			}).Error(err.Error())
		}
		status = httpErrorResponse(err)
	}

	d.Title = fmt.Sprintf("Error %d", status)
	d.Status = status
	w.WriteHeader(status)
	w.Header().Set("Cache-Control", " no-store, must-revalidate")
	w.Header().Set("Pragma", " no-cache")
	w.Header().Set("Expires", " 0")
	h.RenderTemplate(r, w, "error", d)
}
