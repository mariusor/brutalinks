package app

import (
	"encoding/base64"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/gorilla/csrf"
	"github.com/mariusor/littr.go/internal/assets"
	"github.com/mariusor/littr.go/internal/config"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/unrolled/render"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type CtxLogFn func(...log.Ctx) LogFn
type LogFn func(string, ...interface{})

type view struct {
	c      *config.Configuration
	assets assets.AssetFiles
	s      sess
	infoFn CtxLogFn
	errFn  CtxLogFn
}

func ViewInit(c appConfig, infoFn, errFn CtxLogFn) (*view, error) {
	v := new(view)
	v.c = &c.Configuration
	v.infoFn = infoFn
	v.errFn = errFn

	var err error
	v.s, err = initSession(c, infoFn, errFn)
	return v, err
}

func ToTitle(s string) string {
	if len(s) < 1 {
		return s
	}
	return strings.ToUpper(s[0:1]) + s[1:]
}

func pastTenseVerb(s template.HTML) template.HTML {
	l := len(s)
	if l == 0 {
		return ""
	}
	var tensed template.HTML
	if s[len(s)-1] == 'e' {
		tensed = s + "d"
	} else {
		tensed = s + "ed"
	}
	return tensed
}

func renderActivityLabel(r Renderable) template.HTML {
	t := r.Type()

	lbl := "unknown"
	switch t {
	case CommentType:
		lbl = "item"
		if i, ok := r.(*Item); ok {
			if i.IsTop() {
				lbl = "submission"
			} else {
				lbl = "comment"
			}
		}

	case FollowType:
		lbl = "follow"
	case AppreciationType:
		if i, ok := r.(*Vote); ok {
			if i.IsYay() {
				lbl = "like"
			} else {
				lbl = "dislike"
			}
		}
	case ActorType:
		return "account"
	case ModerationType:
		if i, ok := r.(Moderatable); ok {
			if i.IsBlock() {
				lbl = "block"
			} else if i.IsIgnore() {
				lbl = "ignore"
			} else if i.IsReport() {
				lbl = "report"
			} else if i.IsUpdate() {
				lbl = "update"
			} else if i.IsDelete() {
				lbl = "delete"
			}
		}
	}
	return template.HTML(lbl)
}

func genitive(name string) string {
	l := len(name)
	if l == 0 {
		return name
	}
	if name[l-1:l] != "s" {
		return name + "'s"
	}
	return name + "'"
}

func stringInSlice(ss []string) func(v string) bool {
	return func(v string) bool {
		for _, vv := range ss {
			if vv == v {
				return true
			}
		}
		return false
	}
}

func (v *view) RenderTemplate(r *http.Request, w http.ResponseWriter, name string, m Model) error {
	var err error
	var ac *Account

	_, isError := m.(*errorModel)

	layout := "layout"
	accountFromRequest := func() *Account {
		if ac == nil {
			ac = loggedAccount(r)
		}
		return ac
	}

	version := Instance.Version
	ren := render.New(render.Options{
		AssetNames: assets.TemplateNames,
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
			"IsComment":             func(t Renderable) bool { return t.Type() == CommentType },
			"IsFollowRequest":       func(t Renderable) bool { return t.Type() == FollowType },
			"IsVote":                func(t Renderable) bool { return t.Type() == AppreciationType },
			"IsAccount":             func(t Renderable) bool { return t.Type() == ActorType },
			"IsModeration":          func(t Renderable) bool { return t.Type() == ModerationType },
			"LoadFlashMessages":     v.loadFlashMessages(w, r),
			"Mod10":                 mod10,
			"ShowText":              showText(m),
			"HTML":                  html,
			"Text":                  text,
			"isAudio":               isAudio,
			"Audio":                 audio,
			"Video":                 video,
			"isVideo":               isVideo,
			"Image":                 image,
			"isImage":               isImage,
			"Markdown":              Markdown,
			"replaceTags":           replaceTags,
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
			"Config":                func() config.Configuration { return *v.c },
			"Version":               func() string { return version },
			"Name":                  appName,
			"Menu":                  func() []headerEl { return headerMenu(r) },
			"icon":                  icon,
			"svg":                   assets.Svg,
			"js":                    assets.Js,
			"style":                 assets.Style,
			"integrity":             assets.Integrity,
			"req":                   func() *http.Request { return r },
			"url":                   func() url.Values { return r.URL.Query() },
			"urlValue":              func(k string) []string { return r.URL.Query()[k] },
			"urlValueContains":      func(k, v string) bool { return stringInSlice(r.URL.Query()[k])(v) },
			"sameBase":              sameBasePath,
			"sameHash":              HashesEqual,
			"fmtPubKey":             fmtPubKey,
			"pluralize":             func(s string, cnt int) string { return pluralize(float64(cnt), s) },
			"pasttensify":           pastTenseVerb,
			"ShowFollowLink":        func(a *Account) bool { return showFollowLink(accountFromRequest(), a) },
			"ShowAccountBlockLink":  func(a *Account) bool { return showAccountBlockLink(accountFromRequest(), a) },
			"ShowAccountReportLink": func(a *Account) bool { return showAccountReportLink(accountFromRequest(), a) },
			"AccountFollows":        func(a *Account) bool { return AccountFollows(a, accountFromRequest()) },
			"AccountIsFollowed":     func(a *Account) bool { return AccountIsFollowed(accountFromRequest(), a) },
			"AccountIsRejected":     func(a *Account) bool { return AccountIsRejected(accountFromRequest(), a) },
			"AccountIsBlocked":      func(a *Account) bool { return AccountIsBlocked(accountFromRequest(), a) },
			"AccountIsReported":     func(a *Account) bool { return AccountIsReported(accountFromRequest(), a) },
			"ItemReported":          func(i *Item) bool { return ItemIsReported(accountFromRequest(), i) },
			"RenderLabel":           renderActivityLabel,
			csrf.TemplateTag:        func() template.HTML { return csrf.TemplateField(r) },
			"ToTitle":               ToTitle,
			"trimSuffix":            strings.TrimSuffix,
			//"ScoreFmt":          func(i int64) string { return humanize.FormatInteger("#\u202F###", int(i)) },
			//"NumberFmt":         func(i int64) string { return humanize.FormatInteger("#\u202F###", int(i)) },
		}},
		Delims:                    render.Delims{Left: "{{", Right: "}}"},
		Charset:                   "UTF-8",
		DisableCharset:            false,
		BinaryContentType:         "application/octet-stream",
		HTMLContentType:           "text/html",
		IsDevelopment:             Instance.Conf.Env.IsDev(),
		DisableHTTPErrorRendering: true,
	})

	if err = ren.HTML(w, http.StatusOK, name, m); err != nil {
		v.errFn(log.Ctx{"err": err, "model": m})("failed to render template %s", name)
		return errors.Annotatef(err, "failed to render template")
	}
	if !isError {
		if err := v.s.save(w, r); err != nil {
			v.HandleErrors(w, r, err)
			return nil
		}
	}
	return nil
}

func getCSPHashes(m Model, v view) (string, string) {
	var (
		assets    = make([]string, 0)
		styles    = make([]string, 0)
		scripts   = make([]string, 0)
		styleSrc  = "'unsafe-inline'"
		scriptSrc = "'unsafe-inline'"
	)
	if m != nil {
		assets = append(assets, m.Template())
	} else {
		for asset := range v.assets {
			assets = append(assets, asset)
		}
	}
	for _, name := range assets {
		if hash, ok := v.assets.SubresourceIntegrityHash(name); ok {
			ext := path.Ext(name)
			if ext == ".css" {
				styles = append(styles, fmt.Sprintf("sha256-%s", hash))
			} else if ext == ".js" {
				scripts = append(scripts, fmt.Sprintf("sha256-%s", hash))
			}
		}
	}
	if len(styles) > 0 {
		styleSrc = fmt.Sprintf("'%s'", strings.Join(styles, "' '"))
	}
	if len(scripts) > 0 {
		scriptSrc = fmt.Sprintf("'%s'", strings.Join(scripts, "' '"))
	}
	return styleSrc, scriptSrc
}

func (v view) SetCSP(m Model, w http.ResponseWriter) error {
	styleSrc, scriptSrc := getCSPHashes(m, v)
	cspHdrVal := fmt.Sprintf("default-src https: 'self'; style-src https: 'self' %s; script-src https: 'self' %s", styleSrc, scriptSrc)
	w.Header().Set("Content-Security-Policy", cspHdrVal)
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
			v.addFlashMessage(Error, w, r, err.Error())
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
		})("Failed to save session before redirect")
		v.HandleErrors(w, r, err)
		return
	}
	http.Redirect(w, r, url, status)
}

func (v *view) addFlashMessage(typ flashType, w http.ResponseWriter, r *http.Request, msgs ...string) {
	if !v.s.enabled {
		return
	}
	v.s.addFlashMessages(typ, w, r, msgs...)
}

func (v *view) loadFlashMessages(w http.ResponseWriter, r *http.Request) func() []flash {
	var flashData []flash
	flashFn := func() []flash { return flashData }

	s, err := v.s.get(w, r)
	if err != nil || s == nil || !v.c.SessionsEnabled {
		return flashFn
	}
	flashFn, err = v.s.loadFlashMessages(w, r)
	if err != nil {
		v.errFn(log.Ctx{"err": err})("unable to load flash messages")
	}
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
	return data
}

func isAudio(mime string) bool {
	return strings.Contains(mime, "audio")
}

func isVideo(mime string) bool {
	return strings.Contains(mime, "video")
}
func isImage(mime string) bool {
	return strings.Contains(mime, "image")
}

func audio(mime, data string) template.HTML {
	return template.HTML(fmt.Sprintf("<audio src='data:%s;base64,%s'/>", mime, data))
}

func video(mime, data string) template.HTML {
	return template.HTML(fmt.Sprintf("<video  src='data:%s;base64,%s'/>", mime, data))
}

func image(mime, data string) template.HTML {
	if mime == "image/svg+xml" {
		dec, err := base64.StdEncoding.DecodeString(data)
		if err == nil {
			data = string(dec)
		}
		return template.HTML(data)
	}
	return template.HTML(fmt.Sprintf("<image src='data:%s;base64,%s'/>", mime, data))
}

func icon(icon string, c ...string) template.HTML {
	cls := make([]string, 0)
	cls = append(cls, icon)
	cls = append(cls, c...)

	buf := fmt.Sprintf(`<svg aria-hidden="true" class="icon icon-%s"><use xlink:href="#icon-%s"><title>%s</title></use></svg>`,
		strings.Join(cls, " "), icon, icon)

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
