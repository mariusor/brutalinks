package app

import (
	"bufio"
	"bytes"
	"context"
	xerrors "errors"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/gorilla/csrf"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/gorilla/sessions"
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
	conf    appConfig
	v       *view
	logger  log.Logger
	storage *repository
}

var defaultAccount = AnonymousAccount

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
	switch unit {
	case "day":
		fallthrough
	case "hour":
		fallthrough
	case "minute":
		return fmt.Sprintf("%.0f %s %s", val, pluralize(val, unit), when)
	}
	return fmt.Sprintf("%.1f %s %s", val, pluralize(val, unit), when)
}

type appConfig struct {
	Env             EnvType
	Version         string
	APIURL          string
	BaseURL         string
	HostName        string
	Secure          bool
	SessionKeys     [][]byte
	SessionsBackend string
	Logger          log.Logger
}

func Init(c appConfig) (handler, error) {
	var err error

	h := handler{}

	if c.Logger != nil {
		h.logger = c.Logger
	}

	if c.SessionsBackend = os.Getenv("SESSIONS_BACKEND"); c.SessionsBackend == "" {
		c.SessionsBackend = "cookie"
	}
	c.SessionsBackend = strings.ToLower(c.SessionsBackend)
	c.SessionKeys = loadEnvSessionKeys()
	h.v, _ = ViewInit(c)
	h.conf = c

	h.storage = ActivityPubService(c)
	key := os.Getenv("OAUTH2_KEY")
	pw := os.Getenv("OAUTH2_SECRET")
	if len(key) > 0 {
		oIRI := pub.IRI(fmt.Sprintf("%s/actors/%s", h.storage.BaseURL, key))
		oauth, err := h.storage.fedbox.Actor(oIRI)
		if err == nil {
			h.storage.app = new(Account)
			h.storage.app.FromActivityPub(oauth)
			config := GetOauth2Config("fedbox", h.conf.BaseURL)

			handle := h.storage.app.Handle
			tok, err := config.PasswordCredentialsToken(context.Background(), handle, pw)
			if err != nil {
				return h, err
			}
			if tok == nil {
				return h, err
			}
			h.storage.app.Metadata.OAuth.Provider = "fedbox"
			h.storage.app.Metadata.OAuth.Token = tok.AccessToken
			h.storage.app.Metadata.OAuth.TokenType = tok.TokenType
			h.storage.app.Metadata.OAuth.RefreshToken = tok.RefreshToken
		}
	}

	return h, err
}

func initCookieSession(h string, secure bool, k ...[]byte) (sessions.Store, error) {
	ss := sessions.NewCookieStore(k...)
	ss.Options.Domain = h
	ss.Options.Path = "/"
	ss.Options.HttpOnly = true
	ss.Options.Secure = secure
	return ss, nil
}

func initFileSession(h string, secure bool, k ...[]byte) (sessions.Store, error) {
	sessDir := fmt.Sprintf("%s/%s", os.TempDir(), h)
	if _, err := os.Stat(sessDir); os.IsNotExist(err) {
		if err := os.Mkdir(sessDir, 0700); err != nil {
			return nil, err
		}
	}
	ss := sessions.NewFilesystemStore(sessDir, k...)
	ss.Options.Domain = h
	ss.Options.Path = "/"
	ss.Options.HttpOnly = true
	ss.Options.Secure = secure
	return ss, nil
}

func loadScoreFormat(s int) (string, string) {
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

func scoreClass(s int) string {
	_, class := loadScoreFormat(s)
	if class == "" {
		class = "H"
	}
	return class
}

func scoreFmt(s int) string {
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

func account(r *http.Request) *Account {
	acct, ok := ContextAccount(r.Context())
	if !ok {
		return &defaultAccount
	}
	return acct
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

func AccountFollows(a, b *Account) bool {
	for _, acc := range a.Following {
		if HashesEqual(acc.Hash, b.Hash) {
			return true
		}
	}
	return false
}

func AccountIsFollowed(a, b *Account) bool {
	for _, acc := range a.Followers {
		if HashesEqual(acc.Hash, b.Hash) {
			return true
		}
	}
	return false
}

func showFollowedLink(logged, current *Account) bool {
	if !Instance.Config.UserFollowingEnabled {
		return false
	}
	if !logged.IsLogged() {
		return false
	}
	if HashesEqual(logged.Hash, current.Hash) {
		return false
	}
	if AccountFollows(logged, current) {
		return false
	}
	return true
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
		h.v.HandleErrors(w, r, errs...)
		return
	}
	code := q.Get("code")
	state := q.Get("state")
	if len(code) == 0 {
		h.v.HandleErrors(w, r, errors.Forbiddenf("%s error: Empty authentication token", provider))
		return
	}

	conf := GetOauth2Config(provider, h.conf.BaseURL)
	tok, err := conf.Exchange(r.Context(), code)
	if err != nil {
		h.logger.Errorf("%s", err)
		h.v.HandleErrors(w, r, err)
		return
	}

	s, _ := h.v.s.get(r)
	account := loadCurrentAccountFromSession(s, h.storage, h.logger)
	if account.Metadata == nil {
		account.Metadata = &AccountMetadata{}
	}
	account.Metadata.OAuth = OAuth{
		State:        state,
		Code:         code,
		Provider:     provider,
		Token:        tok.AccessToken,
		TokenType:    tok.TokenType,
		RefreshToken: tok.RefreshToken,
		Expiry:       tok.Expiry,
	}

	s.Values[SessionUserKey] = account
	if strings.ToLower(provider) != "local" {
		h.v.addFlashMessage(Success, r, fmt.Sprintf("Login successful with %s", provider))
	} else {
		h.v.addFlashMessage(Success, r, "Login successful")
	}
	h.v.Redirect(w, r, "/", http.StatusFound)
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
	case "fedbox":
		fallthrough
	default:
		apiURL := os.Getenv("API_URL")
		config = oauth2.Config{
			ClientID:     os.Getenv("OAUTH2_KEY"),
			ClientSecret: os.Getenv("OAUTH2_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL:  fmt.Sprintf("%s/oauth/authorize", apiURL),
				TokenURL: fmt.Sprintf("%s/oauth/token", apiURL),
			},
		}
	}
	confOauth2URL := os.Getenv("OAUTH2_URL")
	if u, err := url.Parse(confOauth2URL); err != nil || u.Host == "" {
		config.RedirectURL = fmt.Sprintf("%s/auth/%s/callback", localBaseURL, provider)
	}
	return config
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

func loadCurrentAccountFromSession(s *sessions.Session, r *repository, l log.Logger) Account {
	// load the current account from the session or setting it to anonymous
	raw, ok := s.Values[SessionUserKey]
	if !ok {
		return defaultAccount
	}
	a, ok := raw.(Account)
	if !ok {
		return defaultAccount
	}
	l.WithContext(log.Ctx{
		"handle": a.Handle,
		"hash":   a.Hash,
	}).Debug("loaded account from session")
	return a
}

func loadFlashMessages(r *http.Request, w http.ResponseWriter, s *sessions.Session) func() []flash {
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
	s.Save(r, w)
	return func() []flash { return flashData }
}

func SetSecurityHeaders(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline';")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Xss-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func (h *handler) LoadSession(next http.Handler) http.Handler {
	if !Instance.Config.SessionsEnabled {
		return next
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		h.storage.WithAccount(nil)
		if h.v.s == nil {
			h.logger.Warn("missing session store, unable to load session")
			return
		}
		s, err := h.v.s.get(r)
		if err != nil {
			h.logger.WithContext(log.Ctx{
				"err": err,
			}).Error("unable to load session")
			if xerrors.Is(err, new(os.PathError)) {
				h.v.s.new(r)
			}
		}
		acc := loadCurrentAccountFromSession(s, h.storage, h.logger)
		m := acc.Metadata
		if acc.IsLogged() {
			acc, err = h.storage.LoadAccount(Filters{
				LoadAccountsFilter: LoadAccountsFilter{
					Handle: []string{acc.Handle},
					Key:    []Hash{acc.Hash},
				},
			})
			ctx := log.Ctx{
				"handle": acc.Handle,
				"hash":   acc.Hash,
			}
			if err != nil {
				h.logger.WithContext(ctx).Warn(err.Error())
			}
			// TODO(marius): this needs to be moved to where we're handling all Inbox activities, not on page load
			acc, err = h.storage.loadAccountsFollowers(acc)
			if err != nil {
				h.logger.WithContext(ctx).Warn(err.Error())
			}
			acc, err = h.storage.loadAccountsFollowing(acc)
			if err != nil {
				h.logger.WithContext(ctx).Warn(err.Error())
			}
			// TODO(marius): Fix this ugly hack where we need to not override OAuth2 metadata loaded at login
			acc.Metadata = m
			r = r.WithContext(context.WithValue(r.Context(), AccountCtxtKey, &acc))
			h.storage.WithAccount(&acc)
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func (h *handler) addFlashErrors(r *http.Request, errs ...error) {
	msg := ""
	for _, err := range errs {
		msg += err.Error()
	}
	h.v.addFlashMessage(Error, r, msg)
}

func (h handler) NeedsSessions(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if !Instance.Config.SessionsEnabled {
			h.v.HandleErrors(w, r, errors.NotFoundf("sessions are disabled"))
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

	repo := h.storage
	info, err := repo.LoadInfo()
	if err != nil {
		h.v.HandleErrors(w, r, errors.NewNotValid(err, "oops!"))
		return
	}
	m.Desc.Description = info.Description

	h.v.RenderTemplate(r, w, "about", m)
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
		h.v.HandleErrors(w, r, errs...)
	}
	return http.HandlerFunc(fn)
}

var nodeInfo = WebInfo{}

func getNodeInfo(req *http.Request) (WebInfo, error) {
	c := req.Context()
	repo, ok := ContextRepository(c)
	if !ok {
		err := errors.Errorf("could not load item repository from Context")
		return WebInfo{}, err
	}

	var err error
	if nodeInfo.Title == "" {
		nodeInfo, err = repo.LoadInfo()
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
