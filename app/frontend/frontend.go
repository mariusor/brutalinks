package frontend

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/gorilla/csrf"
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
	"github.com/mariusor/littr.go/internal/errors"
	"github.com/mariusor/littr.go/internal/log"
	"golang.org/x/oauth2"
)

const (
	sessionName = "_s"
	csrfName    = "_c"
	templateDir = "templates/"
	assetsDir   = "assets/"
)

type handler struct {
	conf    Config
	sstor   sessions.Store
	account app.Account
	logger  log.Logger
}

var defaultAccount = app.AnonymousAccount

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

	buf := fmt.Sprintf(`<svg class="icon icon-%s %s"><use xlink:href="#icon-%s" /></svg>`,
		icon, strings.Join(cls, " "), icon)

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

func pluralize(d float64, unit string) string {
	l := len(unit)
	cons := func(c byte) bool {
		cons := []byte{'b', 'c', 'd', 'f', 'g', 'h', 'j', 'k', 'l', 'm', 'n', 'p', 'q', 'r', 's', 't', 'v', 'w', 'y', 'z'}
		for _, cc := range cons {
			if c == cc {
				return true
			}
		}
		return false
	}
	if math.Round(d) != 1 {
		if cons(unit[l-2]) && unit[l-1] == 'y' {
			unit = unit[:l-1] + "ie"
		}
		return unit + "s"
	}
	return unit
}

func relTimeFmt(old time.Time) string {
	td := time.Now().UTC().Sub(old)
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
	Env             app.EnvType
	Version         string
	BaseURL         string
	HostName        string
	Secure          bool
	SessionKeys     [][]byte
	SessionsBackend string
	Logger          log.Logger
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

	if c.SessionsBackend = os.Getenv("SESSIONS_BACKEND"); c.SessionsBackend == "" {
		c.SessionsBackend = "cookie"
	}

	c.SessionKeys = loadEnvSessionKeys()
	h.sstor, err = InitSessionStore(c)
	h.conf = c
	return h, err
}

// InitSessionStore initializes the session store if we have encryption key settings in the env variables
func InitSessionStore(c Config) (sessions.Store, error) {
	var s sessions.Store
	if len(c.SessionKeys) == 0 {
		err := errors.NotImplementedf("no session encryption configuration, unable to use sessions")
		if c.Logger != nil {
			c.Logger.Warn(err.Error())
		}
		//app.Config.SessionsEnabled = false
		return nil, err
	}
	switch strings.ToLower(c.SessionsBackend) {
	case "file":
		sessDir := fmt.Sprintf("%s/%s", os.TempDir(), c.HostName)
		if _, err := os.Stat(sessDir); os.IsNotExist(err) {
			if err := os.Mkdir(sessDir, 0700); err != nil {
				c.Logger.WithContext(log.Ctx{
					"folder": sessDir,
					"err":    err,
				}).Error("unable to create folder")
			}
		}
		ss := sessions.NewFilesystemStore(sessDir, c.SessionKeys...)
		s = ss
	case "cookie":
		fallthrough
	default:
		ss := sessions.NewCookieStore(c.SessionKeys...)
		ss.Options.Domain = c.HostName
		ss.Options.Path = "/"
		ss.Options.HttpOnly = true
		ss.Options.Secure = c.Secure
		s = ss
	}
	return s, nil
}

type errorModel struct {
	Status int
	Title  string
	Errors []error
}

func loadScoreFormat(s int64) (string, string) {
	const (
		ScoreMaxK = 1000.0
		ScoreMaxM = 1000000.0
		ScoreMaxB = 1000000000.0
	)
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
	Auth      bool
	Name      string
	URL       string
}

func headerMenu(r *http.Request) []headerEl {
	sections := []string{"self", "federated", "followed"}
	ret := make([]headerEl, 0)
	for _, s := range sections {
		el := headerEl{
			Name: s,
			URL:  fmt.Sprintf("/%s", s),
		}
		if path.Base(r.URL.Path) == s {
			el.IsCurrent = true
		}
		switch strings.ToLower(s) {
		case "self":
			el.Icon = "home"
		case "federated":
			el.Icon = "activitypub"
		case "followed":
			el.Icon = "star"
			el.Auth = true
		}
		ret = append(ret, el)
	}

	return ret
}

func appName(n string) template.HTML {
	if n == "" {
		return template.HTML(n)
	}
	parts := strings.Split(n, " ")
	name := strings.Builder{}

	initial := parts[0][0:1]
	name.WriteString("<strong>")
	name.WriteString(string(icon(initial)))
	name.WriteString(parts[0][1:])
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

func (h *handler) saveSession(w http.ResponseWriter, r *http.Request) error {
	if h.sstor == nil {
		err := errors.New("missing session store, unable to save session")
		h.logger.Errorf("%s", err)
		return err
	}
	s, err := h.sstor.Get(r, sessionName)
	if err != nil {
		h.logger.Errorf("%s", err)
		return errors.Errorf("failed to load session before redirect: %s", err)
	}
	if err := h.sstor.Save(r, w, s); err != nil {
		err :=errors.Errorf("failed to save session before redirect: %s", err)
		h.logger.Errorf("%s", err)
		return err
	}
	return nil
}

func (h *handler) Redirect(w http.ResponseWriter, r *http.Request, url string, status int) {
	if err := h.saveSession(w, r); err != nil {
		h.logger.WithContext(log.Ctx{
			"status": status,
			"url":    url,
		}).Error(err.Error())
	}

	http.Redirect(w, r, url, status)
}

func sameBasePath(s1 string, s2 string) bool {
	return path.Base(s1) == path.Base(s2)
}

func showText(m interface{}) func() bool {
	return func() bool {
		mm, ok := m.(itemListingModel)
		return !ok || !mm.HideText
	}
}

func sameHash(h1 app.Hash, h2 app.Hash) bool {
	var s1, s2 string
	if len(h1) > len(h2) {
		s1 = string(h1)
		s2 = string(h2)
	} else {
		s1 = string(h2)
		s2 = string(h1)
	}
	return strings.Contains(s1, s2)
}

func fmtPubKey(pub []byte) string {
	s := strings.Builder{}
	eolIdx := 0
	for _, b := range pub {
		if b == '\n' {
			eolIdx = 0
		}
		if eolIdx > 0 && eolIdx%65 == 0 {
			s.WriteByte('\n')
			eolIdx = 1
		}
		s.WriteByte(b)
		eolIdx++
	}
	return s.String()
}

// buildLink("name", someVar1, anotherVar2) :: /path/of/name/{var1}/{var2} -> /path/of/name/someVar1/someVar2
func buildLink(routes chi.Routes, name string, par ...interface{}) string {
	for _, r := range routes.Routes() {
		if strings.Contains(r.Pattern, name) {

		}
	}
	return "/"
}

func (h *handler) RenderTemplate(r *http.Request, w http.ResponseWriter, name string, m interface{}) error {
	var err error
	s, _ := h.sstor.Get(r, sessionName)

	nodeInfo, err := getNodeInfo(r)
	ren := render.New(render.Options{
		Directory:  templateDir,
		Layout:     "layout",
		Extensions: []string{".html"},
		Funcs: []template.FuncMap{{
			//"urlParam":          func(s string) string { return chi.URLParam(r, s) },
			//"get":               func(s string) string { return r.URL.Query().Get(s) },
			"isInverted":        func() bool { return isInverted(r) },
			"sluggify":          sluggify,
			"title":             func(t []byte) string { return string(t) },
			"getProviders":      getAuthProviders,
			"CurrentAccount":    func() app.Account { return h.account },
			"LoadFlashMessages": loadFlashMessages(s),
			"Mod10":             func(lvl uint8) float64 { return math.Mod(float64(lvl), float64(10)) },
			"ShowText":          showText(m),
			"HTML":              html,
			"Text":              text,
			"replaceTags":       replaceTagsInItem,
			"Markdown":          app.Markdown,
			"AccountLocalLink":  AccountLocalLink,
			"AccountPermaLink":  AccountPermaLink,
			"ShowAccountHandle": ShowAccountHandle,
			"ItemLocalLink":     ItemLocalLink,
			"ItemPermaLink":     ItemPermaLink,
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
			"CanPaginate":       canPaginate,
			"Config":            func() app.Config { return app.Instance.Config },
			"Info":              func() app.Info { return nodeInfo },
			"Name":              appName,
			"Menu":              func() []headerEl { return headerMenu(r) },
			"icon":              icon,
			"asset":             func(p string) template.HTML { return template.HTML(asset(p)) },
			"req":               func() *http.Request { return r },
			"sameBase":          sameBasePath,
			"sameHash":          sameHash,
			"fmtPubKey":         fmtPubKey,
			"pluralize":         func(s string, cnt int) string { return pluralize(float64(cnt), s) },
			csrf.TemplateTag:    func() template.HTML { return csrf.TemplateField(r) },
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

	if app.Instance.Config.Env != app.PROD {
		w.Header().Set("Cache-Control", "no-store")
	}
	if err = ren.HTML(w, http.StatusOK, name, m); err != nil {
		new := errors.New("failed to render template")
		h.logger.WithContext(log.Ctx{
			"template": name,
			"model":    fmt.Sprintf("%T", m),
			//"trace":    new.StackTrace(),
			"previous": err.Error(),
		}).Error(new.Error())
		ren.HTML(w, http.StatusInternalServerError, "error", new)
	}
	if err = h.saveSession(w, r); err != nil {
		h.logger.WithContext(log.Ctx{
			"template": name,
			"model":    fmt.Sprintf("%#v", m),
		}).Error(err.Error())
	}
	return err
}

// HandleAdmin serves /admin request
func (h handler) HandleAdmin(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("done!!!"))
}

// HandleCallback serves /auth/{provider}/callback request
func (h *handler) HandleCallback(w http.ResponseWriter, r *http.Request) {
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
		h.HandleErrors(w, r, errs...)
		return
	}
	code := q.Get("code")
	state := q.Get("state")
	if len(code) == 0 {
		h.HandleErrors(w, r, errors.Forbiddenf("%s error: Empty authentication token", provider))
		return
	}

	conf := GetOauth2Config(provider, h.conf.BaseURL)
	tok, err := conf.Exchange(r.Context(), code)
	if err != nil {
		h.logger.Errorf("%s", err)
		h.HandleErrors(w, r, err)
		return
	}
	oauth := app.OAuth{
		State:        state,
		Code:         code,
		Provider:     provider,
		Token:        tok.AccessToken,
		TokenType:    tok.TokenType,
		RefreshToken: tok.RefreshToken,
		Expiry:       tok.Expiry,
	}

	s, _ := h.sstor.Get(r, sessionName)
	h.account = loadCurrentAccountFromSession(s, h.logger)
	s.Values[SessionUserKey] = sessionAccount{
		Handle: h.account.Handle,
		Hash:   []byte(h.account.Hash),
		OAuth:  oauth,
	}
	if strings.ToLower(provider) != "local" {
		h.addFlashMessage(Success, r, fmt.Sprintf("Login successful with %s", provider))
	} else {
		h.addFlashMessage(Success, r, "Login successful")
	}
	h.Redirect(w, r, "/", http.StatusFound)
}

func GetOauth2Config(provider string, localBaseURL string) oauth2.Config {
	var config oauth2.Config
	switch strings.ToLower(provider) {
	case "github":
		config = oauth2.Config{
			ClientID:     os.Getenv("GITHUB_KEY"),
			ClientSecret: os.Getenv("GITHUB_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://github.com/login/oauth/authorize",
				TokenURL: "https://github.com/login/oauth/access_token",
			},
		}
	case "gitlab":
		config = oauth2.Config{
			ClientID:     os.Getenv("GITLAB_KEY"),
			ClientSecret: os.Getenv("GITLAB_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://gitlab.com/login/oauth/authorize",
				TokenURL: "https://gitlab.com/login/oauth/access_token",
			},
		}
	case "facebook":
		config = oauth2.Config{
			ClientID:     os.Getenv("FACEBOOK_KEY"),
			ClientSecret: os.Getenv("FACEBOOK_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://graph.facebook.com/oauth/authorize",
				TokenURL: "https://graph.facebook.com/oauth/access_token",
			},
		}
	case "google":
		config = oauth2.Config{
			ClientID:     os.Getenv("GOOGLE_KEY"),
			ClientSecret: os.Getenv("GOOGLE_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/auth", // access_type=offline
				TokenURL: "https://accounts.google.com/o/oauth2/token",
			},
		}
	case "local":
		config = oauth2.Config{
			ClientID:     os.Getenv("OAUTH2_KEY"),
			ClientSecret: os.Getenv("OAUTH2_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL:  fmt.Sprintf("%s/oauth/authorize", localBaseURL),
				TokenURL: fmt.Sprintf("%s/oauth/token", localBaseURL),
			},
		}
	default:
		config = oauth2.Config{}
	}
	config.RedirectURL = fmt.Sprintf("%s/auth/%s/callback", localBaseURL, provider)
	return config
}

// HandleAuth serves /auth/{provider} request
func (h *handler) HandleAuth(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	indexUrl := "/"
	if strings.ToLower(provider) != "local" && os.Getenv("OAUTH2_KEY") == "" {
		h.logger.WithContext(log.Ctx{
			"provider": provider,
		}).Info("Provider has no credentials set")
		h.Redirect(w, r, indexUrl, http.StatusPermanentRedirect)
		return
	}

	// TODO(marius): generated _CSRF state value to check in h.HandleCallback
	config := GetOauth2Config(provider, h.conf.BaseURL)
	if len(config.ClientID) == 0 {
		s, err := h.sstor.Get(r, sessionName)
		if err != nil {
			h.logger.Debugf(err.Error())
		}
		s.AddFlash("Missing oauth provider")
		h.Redirect(w, r, indexUrl, http.StatusPermanentRedirect)
	}
	// redirURL := "http://brutalinks.git/oauth/authorize?access_type=online&client_id=eaca4839ddf16cb4a5c4ca126db8de5c&redirect_uri=http%3A%2F%2Fbrutalinks.git%2Fauth%2Flocal%2Fcallback&response_type=code&state=state"
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

func loadCurrentAccountFromSession(s *sessions.Session, l log.Logger) app.Account {
	// load the current account from the session or setting it to anonymous
	if raw, ok := s.Values[SessionUserKey]; ok {
		if a, ok := raw.(sessionAccount); ok {
			if acc, err := db.Config.LoadAccount(app.Filters{LoadAccountsFilter: app.LoadAccountsFilter{Handle: []string{a.Handle}}}); err == nil {
				l.WithContext(log.Ctx{
					"handle": acc.Handle,
					"hash":   acc.Hash.String(),
				}).Debug("loaded account from session")
				acc.Metadata.OAuth = a.OAuth
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

func loadFlashMessages(s *sessions.Session) func() []flash  {
	flashData := make([]flash, 0)
	flashes := s.Flashes()
	// setting the local flashData value
	for _, int := range flashes {
		if int == nil {
			continue
		}
		if f, ok := int.(flash); ok {
			flashData = append(flashData, f)
		}
	}
	return func() []flash { return flashData }
}

func (h *handler) LoadSession(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if app.Instance.Config.SessionsEnabled {
			if h.sstor != nil {
				s, err := h.sstor.Get(r, sessionName)
				if err != nil {
					s.Options.MaxAge = -1
					err = s.Save(r, w)
					h.logger.Error(err.Error())
				} else {
					h.account = loadCurrentAccountFromSession(s, h.logger)
				}
			} else {
				h.logger.Warn("missing session store, unable to load session")
			}
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func (h *handler) addFlashMessage(typ flashType, r *http.Request, msgs ...string) {
	s, _ := h.sstor.Get(r, sessionName)
	for _, msg := range msgs {
		n := flash{typ, msg}
		s.AddFlash(n)
	}
}

func (h handler) NeedsSessions(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if !app.Instance.Config.SessionsEnabled {
			h.HandleErrors(w, r, errors.NotFoundf("sessions are disabled"))
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
		h.HandleErrors(w, r, errors.NewNotValid(err, "oops!"))
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

func loadEnvSessionKeys() [][]byte {
	keys := make([][]byte, 0)
	if authKey := []byte(os.Getenv("SESS_AUTH_KEY")); authKey != nil {
		keys = append(keys, authKey)
	}
	if encKey := []byte(os.Getenv("SESS_ENC_KEY")); encKey != nil {
		keys = append(keys, encKey)
	}
	return keys
}

func (h *handler) ErrorHandler(errs ...error) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		h.HandleErrors(w, r, errs...)
	}
	return http.HandlerFunc(fn)
}

// HandleErrors serves failed requests
func (h *handler) HandleErrors(w http.ResponseWriter, r *http.Request, errs ...error) {
	d := errorModel{
		Errors: errs,
	}
	renderErrors := true
	if r.Method == http.MethodPost {
		renderErrors = false
	}

	status := http.StatusInternalServerError
	for _, err := range errs {
		if renderErrors {
			status = httpErrorResponse(err)
		} else {
			h.addFlashMessage(Error, r, err.Error())
		}
	}

	if renderErrors {
		d.Title = fmt.Sprintf("Error %d", status)
		d.Status = status
		w.WriteHeader(status)
		w.Header().Set("Cache-Control", " no-store, must-revalidate")
		w.Header().Set("Pragma", " no-cache")
		w.Header().Set("Expires", " 0")
		h.RenderTemplate(r, w, "error", d)
	} else {
		backURL := "/"
		if refURLs, ok := r.Header["Referer"]; ok {
			backURL = refURLs[0]
		}
		h.Redirect(w, r, backURL, http.StatusFound)
	}
}

var nodeInfo = app.Info{}

func getNodeInfo(req *http.Request) (app.Info, error) {
	c := req.Context()
	nodeInfoLoader, ok := app.ContextNodeInfoLoader(c)
	if !ok {
		err := errors.Errorf("could not load item repository from Context")
		return app.Info{}, err
	}

	var err error
	if nodeInfo.Title == "" {
		nodeInfo, err = nodeInfoLoader.LoadInfo()
	}
	return nodeInfo, err
}

func (h handler) CSRF(next http.Handler) http.Handler {
	opts := []csrf.Option{
		csrf.CookieName(csrfName),
		csrf.FieldName(csrfName),
		csrf.Secure(h.conf.Env.IsProd()),
		csrf.ErrorHandler(h.ErrorHandler(errors.Forbiddenf("Invalid request token"))),
	}
	var authKey []byte
	if len(h.conf.SessionKeys) > 0 {
		authKey = h.conf.SessionKeys[0]
	} else {
		if h.conf.Env.IsProd() {
			h.logger.Warnf("Invalid CSRF auth key")
		}
		// TODO(marius): WTF is this?
		authKey = []byte{0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1}
	}
	return csrf.Protect(authKey, opts...)(next)
}
