package app

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/go-ap/errors"
	"github.com/gorilla/csrf"
	"github.com/gorilla/sessions"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/unrolled/render"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"html/template"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
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

type LogFn func(string, log.Ctx)

type session struct {
	s sessions.Store
}

func (h *session) new(r *http.Request) (*sessions.Session, error) {
	return h.s.New(r, sessionName)
}
func (h *session) get(r *http.Request) (*sessions.Session, error) {
	return h.s.Get(r, sessionName)
}

func (h *session) save(w http.ResponseWriter, r *http.Request) error {
	if h.s == nil {
		err := errors.Newf("missing session store, unable to save session")
		return err
	}
	s, err := h.s.Get(r, sessionName)
	if err != nil {
		return errors.Errorf("failed to load session before redirect: %s", err)
	}
	if err := h.s.Save(r, w, s); err != nil {
		err := errors.Errorf("failed to save session before redirect: %s", err)
		return err
	}
	return nil
}

type view struct {
	s      *session
	infoFn LogFn
	errFn  LogFn
}

func ViewInit(c appConfig, infoFn, errFn LogFn) (*view, error) {
	v := view{
		s:      &session{},
		infoFn: infoFn,
		errFn:  errFn,
	}

	// session encoding for account and flash message objects
	gob.Register(Account{})
	gob.Register(flash{})

	if len(c.SessionKeys) == 0 {
		err := errors.NotImplementedf("no session encryption configuration, unable to use sessions")
		return nil, err
	}
	switch strings.ToLower(c.SessionsBackend) {
	case "file":
		v.s.s, _ = initFileSession(c.HostName, c.Secure, c.SessionKeys...)
	case "cookie":
		fallthrough
	default:
		if strings.ToLower(c.SessionsBackend) != "cookie" {
			v.infoFn(fmt.Sprintf("Invalid session backend %q, falling back to cookie.", c.SessionsBackend), nil)
		}
		v.s.s, _ = initCookieSession(c.HostName, c.Secure, c.SessionKeys...)
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

func (v *view) RenderTemplate(r *http.Request, w http.ResponseWriter, name string, m interface{}) error {
	var err error
	var ac *Account
	var s *sessions.Session

	if s, err = v.s.get(r); err != nil {
		v.errFn(err.Error(), log.Ctx{
			"template": name,
			"model":    m,
		})
	}
	accountFromRequest := func() *Account {
		if ac == nil {
			ac = account(r)
		}
		return ac
	}
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
			"CurrentAccount":    accountFromRequest,
			"IsComment":         func(t Renderable) bool { return t.Type() == Comment },
			"IsFollowRequest":   func(t Renderable) bool { return t.Type() == Follow },
			"IsVote":            func(t Renderable) bool { return t.Type() == Appreciation },
			"LoadFlashMessages": loadFlashMessages(r, w, s),
			"Mod10":             mod10,
			"ShowText":          showText(m),
			"HTML":              html,
			"Text":              text,
			"replaceTags":       replaceTagsInItem,
			"Markdown":          Markdown,
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
			"NumberFmt":         func(i int) string { return numberFormat("%d", i) },
			"TimeFmt":           relTimeFmt,
			"ISOTimeFmt":        isoTimeFmt,
			"ShowUpdate":        showUpdateTime,
			"ScoreClass":        scoreClass,
			"YayLink":           yayLink,
			"NayLink":           nayLink,
			"AcceptLink":        acceptLink,
			"RejectLink":        rejectLink,
			"NextPageLink":      nextPageLink,
			"PrevPageLink":      prevPageLink,
			"CanPaginate":       canPaginate,
			"Config":            func() Configuration { return Instance.Config },
			"Info":              func() WebInfo { return nodeInfo },
			"Name":              appName,
			"Menu":              func() []headerEl { return headerMenu(r) },
			"icon":              icon,
			"asset":             func(p string) template.HTML { return template.HTML(asset(p)) },
			"req":               func() *http.Request { return r },
			"sameBase":          sameBasePath,
			"sameHash":          HashesEqual,
			"fmtPubKey":         fmtPubKey,
			"pluralize":         func(s string, cnt int) string { return pluralize(float64(cnt), s) },
			"ShowFollowLink":    showFollowedLink,
			"Follows":           AccountFollows,
			"IsFollowed":        AccountIsFollowed,
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

	if Instance.Config.Env != PROD {
		w.Header().Set("Cache-Control", "no-store")
	}
	if err = ren.HTML(w, http.StatusOK, name, m); err != nil {
		new := errors.Annotatef(err, "failed to render template")
		v.errFn(new.Error(), log.Ctx{
			"template": name,
			"model":    m,
		})
		ren.HTML(w, http.StatusInternalServerError, "error", new)
	}
	if err = v.s.save(w, r); err != nil {
		v.errFn(err.Error(), log.Ctx{
			"template": name,
			"model":    fmt.Sprintf("%#v", m),
		})
	}
	return err
}

// HandleErrors serves failed requests
func (v *view) HandleErrors(w http.ResponseWriter, r *http.Request, errs ...error) {
	d := errorModel{
		Errors: errs,
	}
	renderErrors := true
	if r.Method == http.MethodPost {
		renderErrors = false
	}
	backURL := "/"
	if refURLs, ok := r.Header["Referer"]; ok {
		backURL = refURLs[0]
		renderErrors = false
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
		d.Title = fmt.Sprintf("Error %d", status)
		d.Status = status
		w.WriteHeader(status)
		w.Header().Set("Cache-Control", " no-store, must-revalidate")
		w.Header().Set("Pragma", " no-cache")
		w.Header().Set("Expires", " 0")
		v.RenderTemplate(r, w, "error", d)
	} else {
		v.Redirect(w, r, backURL, http.StatusFound)
	}
}

func (v *view) Redirect(w http.ResponseWriter, r *http.Request, url string, status int) {
	if err := v.s.save(w, r); err != nil {
		v.errFn(err.Error(), log.Ctx{
			"status": status,
			"url":    url,
		})
	}

	http.Redirect(w, r, url, status)
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

func mod10(lvl uint8) float64 {
	return math.Mod(float64(lvl), float64(10))
}

func showUpdateTime(i Item) bool {
	return !(i.UpdatedAt.IsZero() || math.Abs(float64(i.SubmittedAt.Sub(i.UpdatedAt).Milliseconds())) < float64(2*time.Second))
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

func numberFormat(fmtVerb string, el ...interface{}) string {
	return message.NewPrinter(language.English).Sprintf(fmtVerb, el...)
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

func showText(m interface{}) func() bool {
	return func() bool {
		mm, ok := m.(*listingModel)
		return !ok || !mm.HideText
	}
}

func sluggify(s MimeType) string {
	if s == "" {
		return ""
	}
	return strings.Replace(string(s), "/", "-", -1)
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

func scoreLink(i Item, dir string) string {
	// @todo(marius) :link_generation:
	return fmt.Sprintf("%s/%s", ItemPermaLink(i), dir)
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
func AccountPermaLink(a Account) string {
	if a.HasMetadata() && len(a.Metadata.URL) > 0 {
		return a.Metadata.URL
	}
	return AccountLocalLink(a)
}

// ItemPermaLink
func ItemPermaLink(i Item) string {
	if !i.IsLink() && i.HasMetadata() && len(i.Metadata.URL) > 0 {
		return i.Metadata.URL
	}
	return ItemLocalLink(i)
}

// ItemLocalLink
func ItemLocalLink(i Item) string {
	if i.SubmittedBy == nil {
		// @todo(marius) :link_generation:
		return fmt.Sprintf("/i/%s", i.Hash.Short())
	}
	return fmt.Sprintf("%s/%s", AccountLocalLink(*i.SubmittedBy), i.Hash.Short())
}

func followLink(f FollowRequest) string {
	return fmt.Sprintf("%s/%s", AccountLocalLink(*f.SubmittedBy), "follow")
}

// ShowAccountHandle
func ShowAccountHandle(a Account) string {
	//if strings.Contains(a.Handle, "@") {
	//	// @TODO(marius): simplify this at a higher level in the stack, see Account::FromActivityPub
	//	if parts := strings.SplitAfter(a.Handle, "@"); len(parts) > 1 {
	//		if strings.Contains(parts[1], app.Instance.HostName) {
	//			handle := parts[0]
	//			a.Handle = handle[:len(handle)-1]
	//		}
	//	}
	//}
	return a.Handle
}

func AccountLocalLink(a Account) string {
	handle := "anonymous"
	if len(a.Handle) > 0 {
		handle = a.Handle
	}
	// @todo(marius) :link_generation:
	return fmt.Sprintf("/~%s", handle)
}
