package app

import (
	"encoding/gob"
	"fmt"
	"github.com/go-ap/errors"
	"github.com/gorilla/csrf"
	"github.com/gorilla/sessions"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/unrolled/render"
	"html/template"
	"math"
	"net/http"
	"strings"
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

func ViewInit(c appConfig) (*view, error) {
	v := view{
		s: &session{},
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

func (h *view) addFlashMessage(typ flashType, r *http.Request, msgs ...string) {
	s, _ := h.s.get(r)
	for _, msg := range msgs {
		n := flash{typ, msg}
		s.AddFlash(n)
	}
}

func (h *view) RenderTemplate(r *http.Request, w http.ResponseWriter, name string, m interface{}) error {
	var err error
	var ac *Account
	var s *sessions.Session

	if s, err = h.s.get(r); err != nil {
		h.errFn(err.Error(), log.Ctx{
			"template": name,
			"model":    m,
		})
	}
	nodeInfo, err := getNodeInfo(r)
	ren := render.New(render.Options{
		Directory:  templateDir,
		Layout:     "layout",
		Extensions: []string{".html"},
		Funcs: []template.FuncMap{{
			//"urlParam":          func(s string) string { return chi.URLParam(r, s) },
			//"get":               func(s string) string { return r.URL.Query().Get(s) },
			"isInverted":   func() bool { return isInverted(r) },
			"sluggify":     sluggify,
			"title":        func(t []byte) string { return string(t) },
			"getProviders": getAuthProviders,
			"CurrentAccount": func() *Account {
				if ac == nil {
					ac = account(r)
				}
				return ac
			},
			"IsComment": func(t HasType) bool {
				return t.Type() == Comment
			},
			"IsFollowRequest": func(t HasType) bool {
				return t.Type() == Follow
			},
			"IsVote": func(t HasType) bool {
				return t.Type() == Appreciation
			},
			"LoadFlashMessages": loadFlashMessages(r, w, s),
			"Mod10":             func(lvl uint8) float64 { return math.Mod(float64(lvl), float64(10)) },
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
			"ShowUpdate": func(i Item) bool {
				// TODO(marius): I have to find out why there's a difference between SubmittedAt and UpdatedAt
				//  values coming from fedbox
				return !(i.UpdatedAt.IsZero() || math.Abs(float64(i.SubmittedAt.Sub(i.UpdatedAt).Milliseconds())) < 20000.0)
			},
			"ScoreClass":     scoreClass,
			"YayLink":        yayLink,
			"NayLink":        nayLink,
			"AcceptLink":     acceptLink,
			"RejectLink":     rejectLink,
			"PageLink":       pageLink,
			"CanPaginate":    canPaginate,
			"Config":         func() Configuration { return Instance.Config },
			"Info":           func() WebInfo { return nodeInfo },
			"Name":           appName,
			"Menu":           func() []headerEl { return headerMenu(r) },
			"icon":           icon,
			"asset":          func(p string) template.HTML { return template.HTML(asset(p)) },
			"req":            func() *http.Request { return r },
			"sameBase":       sameBasePath,
			"sameHash":       HashesEqual,
			"fmtPubKey":      fmtPubKey,
			"pluralize":      func(s string, cnt int) string { return pluralize(float64(cnt), s) },
			"ShowFollowLink": showFollowedLink,
			"Follows":        AccountFollows,
			"IsFollowed":     AccountIsFollowed,
			csrf.TemplateTag: func() template.HTML { return csrf.TemplateField(r) },
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
		h.errFn(new.Error(), log.Ctx{
			"template": name,
			"model":    m,
		})
		ren.HTML(w, http.StatusInternalServerError, "error", new)
	}
	if err = h.s.save(w, r); err != nil {
		h.errFn(err.Error(), log.Ctx{
			"template": name,
			"model":    fmt.Sprintf("%#v", m),
		})
	}
	return err
}

// HandleErrors serves failed requests
func (h *view) HandleErrors(w http.ResponseWriter, r *http.Request, errs ...error) {
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
		h.Redirect(w, r, backURL, http.StatusFound)
	}
}

func (h *view) Redirect(w http.ResponseWriter, r *http.Request, url string, status int) {
	if err := h.s.save(w, r); err != nil {
		h.errFn(err.Error(), log.Ctx{
			"status": status,
			"url":    url,
		})
	}

	http.Redirect(w, r, url, status)
}
