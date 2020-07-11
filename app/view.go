package app

import (
	"encoding/gob"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/gorilla/csrf"
	"github.com/gorilla/sessions"
	"github.com/mariusor/littr.go/internal/assets"
	"github.com/mariusor/littr.go/internal/config"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/unrolled/render"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"html/template"
	"math"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

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

type CtxLogFn func(log.Ctx) LogFn
type LogFn func(string, ...interface{})

type session struct {
	s      sessions.Store
	infoFn CtxLogFn
	errFn  CtxLogFn
}

func (s *session) new(r *http.Request) (*sessions.Session, error) {
	return s.s.New(r, sessionName)
}
func (s *session) get(r *http.Request) (*sessions.Session, error) {
	return s.s.Get(r, sessionName)
}

func (s *session) save(w http.ResponseWriter, r *http.Request) error {
	clearSessionCookie := func(w http.ResponseWriter, r *http.Request) {
		if c, _ := r.Cookie(sessionName); c != nil {
			c.Value = ""
			c.MaxAge = 0
			http.SetCookie(w, c)
		}
	}
	if s.s == nil {
		clearSessionCookie(w, r)
		return errors.Newf("missing session store, unable to save session")
	}
	ss, err := s.s.Get(r, sessionName)
	if err != nil {
		clearSessionCookie(w, r)
		return errors.Annotatef(err, "failed to load session")
	}
	if err := s.s.Save(r, w, ss); err != nil {
		clearSessionCookie(w, r)
		return errors.Annotatef(err, "failed to save session")
	}

	s.infoFn(nil)("session saved")
	return nil
}

type view struct {
	s      *session
	infoFn CtxLogFn
	errFn  CtxLogFn
}

func ViewInit(c appConfig, infoFn, errFn CtxLogFn) (*view, error) {
	v := view{
		infoFn: infoFn,
		errFn:  errFn,
	}

	// session encoding for account and flash message objects
	gob.Register(Account{})
	gob.Register(flash{})

	if len(c.SessionKeys) == 0 {
		err := errors.NotImplementedf("no session encryption configuration, unable to use sessions")
		return &v, err
	}
	v.s = &session{
		infoFn: infoFn,
		errFn:  errFn,
	}
	switch strings.ToLower(c.SessionsBackend) {
	case sessionsCookieBackend:
		v.s.s, _ = initCookieSession(c.HostName, c.Env, c.Secure, c.SessionKeys...)
	case sessionsFSBackend:
		fallthrough
	default:
		if strings.ToLower(c.SessionsBackend) != sessionsFSBackend {
			v.infoFn(log.Ctx{"backend": c.SessionsBackend})("Invalid session backend, falling back to %s.", sessionsFSBackend)
			c.SessionsBackend = sessionsFSBackend
		}
		v.s.s, _ = v.initFileSession(c.HostName, c.Env, c.Secure, c.SessionKeys...)
	}
	return &v, nil
}

func (v *view) addFlashMessage(typ flashType, r *http.Request, msgs ...string) {
	s, _ := v.s.get(r)
	for _, msg := range msgs {
		n := flash{typ, msg}
		s.AddFlash(n)
	}
}

func ToTitle(s string) string {
	if len(s) < 1 {
		return s
	}
	return strings.ToUpper(s[0:1]) + s[1:]
}

func renderLabel(r Renderable) template.HTML {
	t := r.Type()

	lbl := "unknown"
	switch t {
	case Comment:
		lbl = "item"
		if i, ok := r.(*Item); ok {
			if i.IsTop() {
				lbl = "submission"
			} else {
				lbl = "comment"
			}
		}

	case Follow:
		lbl = "follow"
	case Appreciation:
		if i, ok := r.(*Vote); ok {
			if i.IsYay() {
				lbl = "like"
			} else {
				lbl = "dislike"
			}
		}
	case Actor:
		return "account"
	case Moderation:
		if i, ok := r.(*ModerationRequest); ok {
			if i.IsBlock() {
				lbl = "block"
			} else if i.IsIgnore() {
				lbl = "ignore"
			} else if i.IsReport() {
				lbl = "report"
			}
		}
	}
	return template.HTML(lbl)
}

func (v *view) RenderTemplate(r *http.Request, w http.ResponseWriter, name string, m Model) error {
	var err error
	var ac *Account
	var s *sessions.Session

	_, isError := m.(*errorModel)

	layout := "layout"
	if Instance.Conf.SessionsEnabled && !isError {
		if s, err = v.s.get(r); err != nil {
			v.errFn(log.Ctx{
				"template": name,
				"model":    m,
			})(err.Error())
		}
	}
	accountFromRequest := func() *Account {
		if ac == nil {
			ac = loggedAccount(r)
		}
		return ac
	}

	ren := render.New(render.Options{
		AssetNames: assets.Files,
		Asset:      assets.Template,
		Layout:     layout,
		Extensions: []string{".html"},
		Funcs: []template.FuncMap{{
			//"urlParam":          func(s string) string { return chi.URLParam(r, s) },
			//"get":               func(s string) string { return r.URL.Query().Get(s) },
			"isInverted":            func() bool { return isInverted(r) },
			"sluggify":              sluggify,
			"title":                 func(t []byte) string { return string(t) },
			"getProviders":          getAuthProviders,
			"CurrentAccount":        accountFromRequest,
			"IsComment":             func(t Renderable) bool { return t.Type() == Comment },
			"IsFollowRequest":       func(t Renderable) bool { return t.Type() == Follow },
			"IsVote":                func(t Renderable) bool { return t.Type() == Appreciation },
			"IsAccount":             func(t Renderable) bool { return t.Type() == Actor },
			"IsModeration":          func(t Renderable) bool { return t.Type() == Moderation },
			"LoadFlashMessages":     loadFlashMessages(r, w, s),
			"Mod10":                 mod10,
			"ShowText":              showText(m),
			"HTML":                  html,
			"Text":                  text,
			"replaceTags":           replaceTagsInItem,
			"Markdown":              Markdown,
			"AccountLocalLink":      AccountLocalLink,
			"AccountPermaLink":      AccountPermaLink,
			"ShowAccountHandle":     ShowAccountHandle,
			"ItemLocalLink":         ItemLocalLink,
			"ItemPermaLink":         PermaLink,
			"PermaLink":             PermaLink,
			"ParentLink":            parentLink,
			"OPLink":                opLink,
			"IsYay":                 isYay,
			"IsNay":                 isNay,
			"ScoreFmt":              scoreFmt,
			"NumberFmt":             func(i int) string { return numberFormat("%d", i) },
			"TimeFmt":               relTimeFmt,
			"ISOTimeFmt":            isoTimeFmt,
			"ShowUpdate":            showUpdateTime,
			"ScoreClass":            scoreClass,
			"YayLink":               yayLink,
			"NayLink":               nayLink,
			"AcceptLink":            acceptLink,
			"RejectLink":            rejectLink,
			"NextPageLink":          nextPageLink,
			"PrevPageLink":          prevPageLink,
			"CanPaginate":           canPaginate,
			"Config":                func() config.Configuration { return *Instance.Conf },
			"Version":               func() string { return Instance.Version },
			"Name":                  appName,
			"Menu":                  func() []headerEl { return headerMenu(r) },
			"icon":                  icon,
			"svg":                   assets.Svg,
			"js":                    assets.Js,
			"req":                   func() *http.Request { return r },
			"sameBase":              sameBasePath,
			"sameHash":              HashesEqual,
			"fmtPubKey":             fmtPubKey,
			"pluralize":             func(s string, cnt int) string { return pluralize(float64(cnt), s) },
			"ShowFollowLink":        func(a *Account) bool { return showFollowLink(accountFromRequest(), a) },
			"ShowAccountBlockLink":  func(a *Account) bool { return showAccountBlockLink(accountFromRequest(), a) },
			"ShowAccountReportLink": func(a *Account) bool { return showAccountReportLink(accountFromRequest(), a) },
			"AccountFollows":        func(a *Account) bool { return AccountFollows(a, accountFromRequest()) },
			"AccountIsFollowed":     func(a *Account) bool { return AccountIsFollowed(accountFromRequest(), a) },
			"AccountIsRejected":     func(a *Account) bool { return AccountIsRejected(accountFromRequest(), a) },
			"AccountIsBlocked":      func(a *Account) bool { return AccountIsBlocked(accountFromRequest(), a) },
			"AccountIsReported":     func(a *Account) bool { return AccountIsReported(accountFromRequest(), a) },
			"ItemReported":          func(i *Item) bool { return ItemIsReported(accountFromRequest(), i) },
			"RenderLabel":           renderLabel,
			csrf.TemplateTag:        func() template.HTML { return csrf.TemplateField(r) },
			"ToTitle":               ToTitle,
			//"ScoreFmt":          func(i int64) string { return humanize.FormatInteger("#\u202F###", int(i)) },
			//"NumberFmt":         func(i int64) string { return humanize.FormatInteger("#\u202F###", int(i)) },
		}},
		Delims:                    render.Delims{Left: "{{", Right: "}}"},
		Charset:                   "UTF-8",
		DisableCharset:            false,
		BinaryContentType:         "application/octet-stream",
		HTMLContentType:           "text/html",
		IsDevelopment:             !Instance.Conf.Env.IsProd(),
		DisableHTTPErrorRendering: true,
	})

	if err = ren.HTML(w, http.StatusOK, name, m); err != nil {
		return errors.Annotatef(err, "failed to render template")
	}
	if Instance.Conf.SessionsEnabled && !isError {
		if err := v.s.save(w, r); err != nil {
			return err
		}
	}
	return nil
}

// HandleErrors serves failed requests
func (v *view) HandleErrors(w http.ResponseWriter, r *http.Request, errs ...error) {
	d := &errorModel{
		Errors: errs,
	}
	renderErrors := true
	if r.Method == http.MethodPost {
		renderErrors = false
	}
	backURL := "/"
	if refURLs, ok := r.Header["Referer"]; ok {
		backURL = refURLs[0]
		//renderErrors = false
	}

	status := http.StatusInternalServerError
	for _, err := range errs {
		if err == nil {
			continue
		}
		if renderErrors {
			status = httpErrorResponse(err)
		} else {
			v.addFlashMessage(Error, r, err.Error())
		}
	}

	if renderErrors {
		d.Status = status
		d.Title = fmt.Sprintf("Error %d", status)
		d.StatusText = http.StatusText(d.Status)
		w.Header().Set("Cache-Control", " no-store, must-revalidate")
		w.Header().Set("Pragma", " no-cache")
		w.Header().Set("Expires", " 0")
		w.WriteHeader(status)
		v.RenderTemplate(r, w, "error", d)
	} else {
		v.Redirect(w, r, backURL, http.StatusFound)
	}
}

func (v *view) Redirect(w http.ResponseWriter, r *http.Request, url string, status int) {
	if err := v.s.save(w, r); err != nil {
		err := errors.Annotatef(err, "Failed to save session before redirect")
		v.errFn(log.Ctx{
			"status": status,
			"url":    url,
		})("Error: %s", err)
		v.HandleErrors(w, r, err)
		return
	}
	http.Redirect(w, r, url, status)
}

func loadFlashMessages(r *http.Request, w http.ResponseWriter, s *sessions.Session) func() []flash {
	var flashData []flash
	flashFn := func() []flash { return flashData }
	if s == nil || !Instance.Conf.SessionsEnabled {
		return flashFn
	}
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
	return flashFn
}

func mod10(lvl uint8) float64 {
	return math.Mod(float64(lvl), float64(10))
}

const minShowUpdateTime = 2 * time.Second

func showUpdateTime(i Renderable) bool {
	if it, ok := i.(*Item); ok {
		return !it.UpdatedAt.IsZero() && it.UpdatedAt.Sub(it.SubmittedAt) > minShowUpdateTime
	}
	if a, ok := i.(*Account); ok {
		return !a.UpdatedAt.IsZero() && a.UpdatedAt.Sub(a.CreatedAt) > minShowUpdateTime
	}
	return false
}

func scoreClass(s int) string {
	_, class := loadScoreFormat(s)
	if class == "" {
		class = "H"
	}
	if s < 0 {
		class += " N"
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

func numberFormat(fmtVerb string, el ...interface{}) string {
	return message.NewPrinter(language.English).Sprintf(fmtVerb, el...)
}

func loadScoreFormat(s int) (string, string) {
	const (
		ScoreMaxK = 1000.0
		ScoreMaxM = 1000000.0
		ScoreMaxB = 1000000000.0
		dK        = 4.0
		dM        = 7.0
		dB        = 10.0
	)
	score := 0.0
	units := ""
	base := float64(s)
	d := math.Ceil(math.Log10(math.Abs(base)))
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

	name.WriteString(string(icon("trash-o")))
	name.WriteString("<strong>")
	name.WriteString(parts[0])
	name.WriteString("</strong>")
	for _, p := range parts[1:] {
		name.WriteString(" <small>")
		name.WriteString(p)
		name.WriteString("</small>")
	}

	return template.HTML(name.String())
}

func showText(m Model) func() bool {
	return func() bool {
		if mm, ok := m.(*listingModel); ok {
			return mm.ShowText
		}
		if _, ok := m.(*contentModel); ok {
			return true
		}
		return true
	}
}

func sluggify(s string) string {
	if s == "" {
		return ""
	}
	return strings.Replace(s, "/", "-", -1)
}

func getAuthProviders() map[string]string {
	p := make(map[string]string)
	if os.Getenv("GITHUB_KEY") != "" {
		p["github"] = "Github"
	}
	if os.Getenv("GITLAB_KEY") != "" {
		p["gitlab"] = "Gitlab"
	}
	if os.Getenv("GOOGLE_KEY") != "" {
		p["google"] = "Google"
	}
	if os.Getenv("FACEBOOK_KEY") != "" {
		p["facebook"] = "Facebook"
	}

	return p
}

func html(data string) template.HTML {
	return template.HTML(data)
}

func text(data string) string {
	return string(data)
}

func icon(icon string, c ...string) template.HTML {
	cls := make([]string, 0)
	cls = append(cls, icon)
	cls = append(cls, c...)

	buf := fmt.Sprintf(`<svg aria-hidden="true" class="icon icon-%s"><use xlink:href="#icon-%s" /></svg>`,
		strings.Join(cls, " "), icon)

	return template.HTML(buf)
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

func scoreLink(i Item, dir string) string {
	// @todo(marius) :link_generation:
	return fmt.Sprintf("%s/%s", ItemPermaLink(&i), dir)
}

func yayLink(i Item) string {
	return scoreLink(i, "yay")
}

func nayLink(i Item) string {
	return scoreLink(i, "nay")
}

func acceptLink(f FollowRequest) string {
	return fmt.Sprintf("%s/%s", followLink(f), "accept")
}

func rejectLink(f FollowRequest) string {
	return fmt.Sprintf("%s/%s", followLink(f), "reject")
}

func nextPageLink(p Hash) template.HTML {
	if len(p) > 0 {
		return template.HTML(fmt.Sprintf("?after=%s", p))
	}
	return ""
}

func prevPageLink(p Hash) template.HTML {
	if len(p) > 0 {
		return template.HTML(fmt.Sprintf("?before=%s", p))
	}
	return ""
}

func canPaginate(m interface{}) bool {
	_, ok := m.(Paginator)
	return ok
}

func sameBasePath(s1 string, s2 string) bool {
	return path.Base(s1) == path.Base(s2)
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

func InOutbox(a *Account, b pub.Item) bool {
	if !a.HasMetadata() {
		return false
	}
	return a.Metadata.outbox.Contains(b)
}

func AccountFollows(a, by *Account) bool {
	for _, acc := range a.Following {
		if HashesEqual(acc.Hash, by.Hash) {
			return true
		}
	}
	return false
}

func AccountBlocks(by, b *Account) bool {
	for _, acc := range by.Blocked {
		if HashesEqual(acc.Hash, b.Hash) {
			return true
		}
	}
	return false
}

func AccountIgnores(by, b *Account) bool {
	for _, acc := range by.Ignored {
		if HashesEqual(acc.Hash, b.Hash) {
			return true
		}
	}
	return false
}

func AccountIsFollowed(a, by *Account) bool {
	for _, acc := range a.Followers {
		if HashesEqual(acc.Hash, by.Hash) {
			return true
		}
	}
	return false
}

func AccountIsRejected(by, a *Account) bool {
	return InOutbox(by, pub.Block{
		Type:   pub.BlockType,
		Object: a.pub.GetLink(),
	})
}

func AccountIsBlocked(by, a *Account) bool {
	for _, acc := range by.Blocked {
		if HashesEqual(acc.Hash, a.Hash) {
			return true
		}
	}
	return false
}

func AccountIsIgnored(by, a *Account) bool {
	for _, acc := range by.Ignored {
		if HashesEqual(acc.Hash, a.Hash) {
			return true
		}
	}
	return false
}

func AccountIsReported(by, a *Account) bool {
	return false
}

func ItemIsReported(by *Account, i *Item) bool {
	return InOutbox(by, pub.Flag{
		Type:   pub.FlagType,
		Object: i.pub.GetLink(),
	})
}

func showAccountBlockLink(by, current *Account) bool {
	if !Instance.Conf.ModerationEnabled {
		return false
	}
	if !by.IsLogged() {
		return false
	}
	if HashesEqual(by.Hash, current.Hash) {
		return false
	}
	if InOutbox(by, pub.Block{
		Type:   pub.BlockType,
		Object: current.pub.GetLink(),
	}) {
		return false
	}
	if AccountBlocks(by, current) {
		return false
	}
	return true
}

func showFollowLink(by, current *Account) bool {
	if !Instance.Conf.UserFollowingEnabled {
		return false
	}
	if !by.IsLogged() {
		return false
	}
	if HashesEqual(by.Hash, current.Hash) {
		return false
	}
	if InOutbox(by, pub.Follow{
		Type:   pub.FollowType,
		Object: current.pub.GetLink(),
	}) {
		return false
	}
	if AccountFollows(by, current) {
		return false
	}
	return true
}

func showAccountReportLink(by, current *Account) bool {
	if !Instance.Conf.ModerationEnabled {
		return false
	}
	if !by.IsLogged() {
		return false
	}
	if HashesEqual(by.Hash, current.Hash) {
		return false
	}
	if InOutbox(by, pub.Block{
		Type:   pub.FlagType,
		Object: current.pub.GetLink(),
	}) {
		return false
	}
	return true
}

func isYay(v *Vote) bool {
	return v != nil && v.Weight > 0
}

func isNay(v *Vote) bool {
	return v != nil && v.Weight < 0
}

func parentLink(c Item) string {
	if c.Parent != nil {
		// @todo(marius) :link_generation:
		return fmt.Sprintf("/i/%s", c.Parent.Hash)
	}
	return ""
}

func opLink(c Item) string {
	if c.OP != nil {
		// @todo(marius) :link_generation:
		return fmt.Sprintf("/i/%s", c.OP.Hash)
	}
	return ""
}

// AccountPermaLink
func AccountPermaLink(a *Account) string {
	if a == nil {
		return ""
	}
	if a.HasMetadata() && len(a.Metadata.URL) > 0 && a.Metadata.URL != a.Metadata.ID {
		return a.Metadata.URL
	}
	return AccountLocalLink(a)
}

// ItemPermaLink
func ItemPermaLink(i *Item) string {
	if i == nil {
		return ""
	}
	if !i.IsLink() && i.HasMetadata() && len(i.Metadata.URL) > 0 {
		return i.Metadata.URL
	}
	return ItemLocalLink(i)
}

// PermaLink
func PermaLink(r Renderable) string {
	if i, ok := r.(*Item); ok {
		return ItemPermaLink(i)
	}
	if a, ok := r.(*Account); ok {
		return AccountPermaLink(a)
	}
	return ""
}

// ItemLocalLink
func ItemLocalLink(i *Item) string {
	if i.SubmittedBy == nil {
		// @todo(marius) :link_generation:
		return fmt.Sprintf("/i/%s", i.Hash.Short())
	}
	return fmt.Sprintf("%s/%s", AccountLocalLink(i.SubmittedBy), i.Hash.Short())
}

func followLink(f FollowRequest) string {
	return fmt.Sprintf("%s/%s", AccountLocalLink(f.SubmittedBy), "follow")
}

// ShowAccountHandle
func ShowAccountHandle(a *Account) string {
	//if strings.Contains(a.Handle, "@") {
	//	// @TODO(marius): simplify this at a higher level in the stack, see Account::FromActivityPub
	//	if parts := strings.SplitAfter(a.Handle, "@"); len(parts) > 1 {
	//		if strings.Contains(parts[1], app.Instance.HostName) {
	//			handle := parts[0]
	//			a.Handle = handle[:len(handle)-1]
	//		}
	//	}
	//}
	handle := Anonymous
	if len(a.Handle) > 0 {
		handle = a.Handle
	}
	return handle
}

func AccountLocalLink(a *Account) string {
	// @todo(marius) :link_generation:
	return fmt.Sprintf("/~%s", ShowAccountHandle(a))
}
