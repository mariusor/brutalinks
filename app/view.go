package app

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"math"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/gorilla/csrf"
	"github.com/mariusor/go-littr/internal/assets"
	"github.com/mariusor/go-littr/internal/config"
	"github.com/mariusor/go-littr/internal/log"
	"github.com/mariusor/qstring"
	"github.com/unrolled/render"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
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

	_, isError := m.(*errorModel)

	layout := "layout"
	acc := loggedAccount(r)
	accountFromRequest := func() *Account { return acc }

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
			"SessionEnabled":        func() bool { return v.s.enabled },
			"LoadFlashMessages":     v.loadFlashMessages(w, r),
			"Mod10":                 mod10,
			"ShowText":              showText(m),
			"ShowTitle":             showTitle(m),
			"HTML":                  html,
			"Text":                  text,
			"isAudio":               isAudio,
			"Audio":                 audio,
			"Video":                 video,
			"isVideo":               isVideo,
			"Image":                 image,
			"Avatar":                avatar,
			"isImage":               isImage,
			"Markdown":              Markdown,
			"replaceTags":           replaceTags,
			"outputTag":             func(t Tag) template.HTML { return template.HTML(mimeTypeTagReplace("text/html", t))},
			"AccountLocalLink":      AccountLocalLink,
			"ShowAccountHandle":     ShowAccountHandle,
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
			"icons":                 icons,
			"svg":                   assets.Svg,
			"js":                    assets.Js,
			"style":                 assets.Style,
			"integrity":             assets.Integrity,
			"req":                   func() *http.Request { return r },
			"url":                   func() url.Values { return r.URL.Query() },
			"urlValue":              func(k string) []string { return r.URL.Query()[k] },
			"urlValueContains":      func(k, v string) bool { return stringInSlice(r.URL.Query()[k])(v) },
			"sameBase":              sameBasePath,
			"sameHash":              func(h1, h2 Hash) bool { return h1 == h2 },
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
			"itemType":              itemType,
			"trimSuffix":            strings.TrimSuffix,
			"Sort": func(list RenderableList) []Renderable {
				if list == nil {
					return nil
				}
				if lModel, ok := m.(*listingModel); ok {
					if lModel.sortFn == nil {
						return ByDate(list)
					}
					return lModel.sortFn(list)
				}
				return nil
			},
			"GetDomainURL":   GetDomainURL,
			"GetDomainTitle": GetDomainTitle,
			//"ScoreFmt":          func(i int64) string { return humanize.FormatInteger("#\u202F###", int(i)) },
			//"NumberFmt":         func(i int64) string { return humanize.FormatInteger("#\u202F###", int(i)) },
			"invitationLink": GetInviteLink(v),
		}},
		Delims:                    render.Delims{Left: "{{", Right: "}}"},
		Charset:                   "UTF-8",
		DisableCharset:            false,
		BinaryContentType:         "application/octet-stream",
		HTMLContentType:           "text/html",
		IsDevelopment:             Instance.Conf.Env.IsDev(),
		DisableHTTPErrorRendering: true,
	})

	if !isError {
		if acc.IsLogged() {
			if err = v.saveAccountToSession(w, r, acc); err != nil {
				v.errFn(log.Ctx{"err": err.Error()})("adding user to session failed")
			}
		}
		if err = v.s.save(w, r); err != nil {
			v.errFn(log.Ctx{"err": err.Error()})("session save failed")
		}
	}
	if err = ren.HTML(w, http.StatusOK, name, m); err != nil {
		v.errFn(log.Ctx{"err": err, "model": m})("failed to render template %s", name)
		return errors.Annotatef(err, "failed to render template")
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
	cspHdrVal := fmt.Sprintf("default-src https: 'self'; style-src https: 'self' %s; script-src https: 'self' %s; media-src https: data: 'self'; img-src https: data: 'self'", styleSrc, scriptSrc)
	w.Header().Set("Content-Security-Policy", cspHdrVal)
	return nil
}

// RedirectToErrors redirects failed requests with a flash error
func (v *view) RedirectToErrors(w http.ResponseWriter, r *http.Request, errs ...error) {
	backURL := "/"
	if refURLs, ok := r.Header["Referer"]; ok {
		backURL = refURLs[0]
	}
	if strings.Contains(backURL, r.RequestURI) {
		backURL = "/"
	}

	for _, err := range errs {
		if err == nil {
			continue
		}
		v.addFlashMessage(Error, w, r, err.Error())
	}

	v.infoFn(log.Ctx{"URL": backURL})("redirecting")
	v.Redirect(w, r, backURL, http.StatusFound)
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
	if url == r.RequestURI {
		url, _ = path.Split(url)
	}
	if err := v.s.save(w, r); err != nil {
		v.errFn(log.Ctx{
			"status": status,
			"url":    url,
			"err":    err.Error(),
		})("Failed to save session before redirect")
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
	if err != nil || s == nil || !v.s.enabled {
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

type headerEl struct {
	IsCurrent bool
	Icon      []string
	Auth      bool
	Name      string
	URL       string
}

func headerMenu(r *http.Request) []headerEl {
	sections := []string{"/self", "/federated", "/followed", "submit"}
	ret := make([]headerEl, 0)
	for _, s := range sections {
		el := headerEl{
			Name: s,
			URL:  fmt.Sprintf("/%s", strings.Trim(s, "/")),
		}
		if path.Base(r.URL.Path) == path.Base(s) {
			el.IsCurrent = true
		}
		switch strings.ToLower(s) {
		case "/self":
			el.Icon = []string{"home"}
		case "/federated":
			el.Icon = []string{"activitypub"}
		case "/followed":
			el.Icon = []string{"star"}
			el.Auth = true
		case "submit":
			el.Icon = []string{"edit", "v-mirror"}
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

func showTitle(m Model) func(i Item) bool {
	return func(i Item) bool {
		switch mm := m.(type) {
		case *listingModel:
			if i.Private() {
				return len(i.Title) > 0
			}
			return !mm.ShowText || i.Parent == nil
		case *contentModel:
			return len(i.Title) > 0 && i.Parent == nil
		}
		return true
	}
}

func showText(m Model) func() bool {
	return func() bool {
		switch mm := m.(type) {
		case *listingModel:
			return mm.ShowText
		case *contentModel:
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

func isDocument(mime string) bool {
	return strings.Contains(mime, "url")
}

func audio(mime, data string) template.HTML {
	return template.HTML(fmt.Sprintf(audioFmt, mime, data, mime))
}

func itemType(mime string) template.HTML {
	t := "item"
	if isVideo(mime) {
		t = "video"
	}
	if isAudio(mime) {
		t = "audio"
	}
	if isImage(mime) {
		t = "image"
	}
	return template.HTML(t)
}

func video(mime, data string) template.HTML {
	return template.HTML(fmt.Sprintf(videoFmt, mime, data, mime))
}

func avatar(typ, data string) template.HTML {
	if m, _, err := mime.ParseMediaType(typ); err == nil {
		typ = m
	}
	if typ == MimeTypeSVG {
		if dec, err := base64.RawStdEncoding.DecodeString(data); err == nil {
			data = string(dec)
		}
		return template.HTML(data)
	}
	return template.HTML(fmt.Sprintf(avatarFmt, typ, data))
}

func image(mime, data string) template.HTML {
	if mime == MimeTypeSVG {
		if dec, err := base64.RawStdEncoding.DecodeString(data); err == nil {
			data = string(dec)
		}
		return template.HTML(data)
	}
	return template.HTML(fmt.Sprintf(imageFmt, mime, data))
}

func icons(c []string) template.HTML {
	return icon(c...)
}

func accountDefaultAvatar (act *Account) ImageMetadata {
	if len(act.Handle) == 0 {
		return ImageMetadata{}
	}
	initial := act.Handle[0:1]
	img := fmt.Sprintf(avatarSvgFmt, "#000", "#fff", 28, initial)
	return ImageMetadata{
		URI:      img,
		MimeType: MimeTypeSVG,
	}
}

const (
	imageFmt     = `<image src='data:%s;base64,%s' />`
	avatarFmt    = `<image src='data:%s;base64,%s' width='48' height='48' class='icon avatar' />`
	videoFmt     = `<video controls><source src='data:%s;base64,%s' type='%s'/></video>`
	audioFmt     = `<audio controls><source src='data:%s;base64,%s' type='%s'/></audio>`
	iconFmt      = `<svg aria-hidden="true" class="icon icon-%s"><use xlink:href="#icon-%s"><title>%s</title></use></svg>`
	avatarSvgFmt = `<svg aria-hidden="true" class="icon avatar" width="48" height="48" viewBox="0 0 50 50">
  <rect width="100%%" height="100%%" fill="%s"/> <text fill="%s" font-size="%d" font-weight="800" x="50%%" y="55%%" dominant-baseline="middle" text-anchor="middle">%s</text>
</svg> `
)

func icon(c ...string) template.HTML {
	if len(c) == 0 {
		return ""
	}
	buf := fmt.Sprintf(iconFmt, strings.Join(c, " "), c[0], c[0])

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
	return path.Join(ItemLocalLink(&i), dir)
}

func yayLink(i Item) string {
	return scoreLink(i, "yay")
}

func nayLink(i Item) string {
	return scoreLink(i, "nay")
}

func acceptLink(f FollowRequest) string {
	return path.Join(followLink(f), "accept")
}

func rejectLink(f FollowRequest) string {
	return path.Join(followLink(f), "reject")
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

func InOutbox(a *Account, it ...pub.Item) bool {
	if !a.HasMetadata() {
		return false
	}
	for _, b := range it {
		if a.Metadata.Outbox.Contains(b) {
			return true
		}
	}
	return false
}

func AccountFollows(a, by *Account) bool {
	return a.Following.Contains(*by)
}

func AccountBlocks(by, b *Account) bool {
	return by.Blocked.Contains(*b)
}

func AccountIgnores(by, b *Account) bool {
	return by.Ignored.Contains(*b)
}

func AccountIsFollowed(a, by *Account) bool {
	return a.Followers.Contains(*by)
}

func AccountIsRejected(by, a *Account) bool {
	return InOutbox(by, pub.Block{
		Type:   pub.BlockType,
		Object: a.pub.GetLink(),
	})
}

func AccountIsBlocked(by, a *Account) bool {
	return by.Blocked.Contains(*a)
}

func AccountIsIgnored(by, a *Account) bool {
	return by.Ignored.Contains(*a)
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
	if by.Hash == current.Hash {
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
	if by.Hash == current.Hash {
		return false
	}
	b := pub.Block{
		Type:   pub.BlockType,
		Object: current.pub.GetLink(),
	}
	f := pub.Follow{
		Type:   pub.FollowType,
		Object: current.pub.GetLink(),
	}
	if InOutbox(by, f, b) {
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
	if by.Hash == current.Hash {
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
	if (!i.IsLink() || i.IsFederated()) && i.HasMetadata() && len(i.Metadata.URL) > 0 {
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
	if i.SubmittedBy == nil || i.SubmittedBy.Handle == Anonymous || i.SubmittedBy.Handle == ""  || i.SubmittedBy.IsFederated() {
		return path.Join("/", i.SubmittedAt.UTC().Format("2006/01/02"), i.Hash.String())
	}
	return path.Join(AccountLocalLink(i.SubmittedBy), i.Hash.String())
}

func followLink(f FollowRequest) string {
	return path.Join(AccountLocalLink(f.SubmittedBy), "follow")
}

// ShowAccountHandle
func ShowAccountHandle(a *Account) string {
	handle := Anonymous
	if len(a.Handle) > 0 {
		handle = a.Handle
	}
	if a.IsFederated() {
		var h string
		if a.HasMetadata() && a.Metadata.URL != "" {
			h = host(a.Metadata.URL)
		} else if a.pub != nil {
			h = host(a.pub.GetLink().String())
		}
		return handle + "@" + h
	}
	return handle
}

func AccountLocalLink(a *Account) string {
	// @todo(marius) :link_generation:
	return fmt.Sprintf("/~%s", ShowAccountHandle(a))
}

const (
	unknownDomain = "unknown"
	githubDomain = "github.com"
	gitlabDomain = "gitlab.com"
	twitchDomain = "twitch.tv"
	twitterDomain = "twitter.com"
)

var twitchValidUser = func(n string) bool {
	return !(stringInSlice([]string{ "directory", "p", "downloads", "jobs", "store", "turbo" })(n))
}

var githubValidUser = func(n string) bool {
	return !(stringInSlice([]string{ "features", "security", "team", "enterprise", "topics", "collections",
		"trending", "events", "marketplace", "pricing", "nonprofit", "join", "contact", "about", "site", "git-guides",
		"discussions", "pulls", "issues", "explore", "settings", "mine", "new", "import", "organizations",
	})(n))
}

var gitlabValidUser = func(n string) bool {
	return !(stringInSlice([]string{"users", "explore", "-", "dashboard", "help" })(n))
}

var twitterValidUser = func(n string) bool {
	return !(stringInSlice([]string{ "home", "explore", "notifications", "messages", "bookmarks", "settings", "i",
		"compose", "search", "tos", "privacy",
	})(n))
}

func getDomain(u *url.URL) string {
	if u == nil || len(u.Host) == 0 {
		return unknownDomain
	}
	pathEl := strings.Split(strings.TrimLeft(u.Path, "/"), "/")
	if len(pathEl) > 0 {
		maybeUser := pathEl[0]
		switch u.Host {
		case twitterDomain, "www." + twitterDomain:
			if twitterValidUser(maybeUser) {
				return fmt.Sprintf("%s/%s", u.Host, maybeUser)
			}
		case gitlabDomain, "www."+gitlabDomain:
			if gitlabValidUser(maybeUser) {
				return fmt.Sprintf("%s/%s", u.Host, maybeUser)
			}
		case githubDomain, "www." + githubDomain:
			if githubValidUser(maybeUser) {
				return fmt.Sprintf("%s/%s", u.Host, maybeUser)
			}
		case twitchDomain, "www." + twitchDomain:
			if twitchValidUser(maybeUser) {
				return fmt.Sprintf("%s/%s", u.Host, maybeUser)
			}
		}
		if len(maybeUser) > 0 && maybeUser[0] == '~' {
			// NOTE(marius): this handles websites that use ~user for home directories
			// Eg, SourceHut, and other Brutalinks instances
			if u.Host == Instance.BaseURL {
				// TODO(marius): I need to generate local user link here instead of /d/$u.Host/$maybeUser
			}
			return fmt.Sprintf("%s/%s", u.Host, maybeUser)
		}
	}
	return u.Host
}

func GetDomainTitle (i Item) template.HTML {
	if !i.IsLink() {
		return unknownDomain
	}
	u, err := url.Parse(i.Data)
	if err != nil {
		return unknownDomain
	}
	return template.HTML(getDomain(u))
}

func GetDomainURL(i Item) template.HTMLAttr {
	if !i.IsLink() {
		return unknownDomain
	}
	u, err := url.Parse(i.Data)
	if err != nil {
		return unknownDomain
	}
	return template.HTMLAttr(url.PathEscape(getDomain(u)))
}

func GetInviteLink(v *view) func(invitee *Account) template.HTMLAttr {
	return func(invitee *Account) template.HTMLAttr {
		u := fmt.Sprintf("%s/register/%s", Instance.BaseURL, invitee.Hash)
		// @todo(marius): :link_generation:
		bodyFmt := "Hello,\n\nThis is an invitation to join %s.\n\nTo accept this invitation and create an account, visit the URL below: %s\n\n/%s"
		mailContent := struct {
			Subject string `qstring:subject`
			Body    string `qstring:body`
		}{
			Subject: fmt.Sprintf("You are invited to join %s", v.c.HostName),
			Body:    fmt.Sprintf(bodyFmt, Instance.BaseURL, u, invitee.CreatedBy.Handle),
		}
		q, _ := qstring.Marshal(&mailContent)
		return template.HTMLAttr(fmt.Sprintf("mailto:?%s", q.Encode()))
	}
}

func (v *view) RedirectWithFailMessage(successFn func () (bool, string)) func (http.Handler) http.Handler {
	return func (next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if success, failMsg := successFn(); !success {
				v.addFlashMessage(Error, w, r, failMsg)
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
