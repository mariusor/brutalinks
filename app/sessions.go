package app

import (
	"encoding/gob"
	"fmt"
	"github.com/go-ap/errors"
	"github.com/gorilla/sessions"
	"github.com/mariusor/littr.go/internal/log"
	"net/http"
	"os"
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

type sess struct {
	enabled bool
	s       sessions.Store
	infoFn  CtxLogFn
	errFn   CtxLogFn
}

func initSession(c appConfig, infoFn, errFn CtxLogFn) (sess, error) {
	// session encoding for account and flash message objects
	gob.Register(Account{})
	gob.Register(flash{})

	if len(c.SessionKeys) == 0 {
		return sess{}, errors.NotImplementedf("no session encryption keys, unable to use sessions")
	}
	s := sess{
		enabled: c.SessionsEnabled,
		infoFn:  infoFn,
		errFn:   errFn,
	}

	var err error
	switch strings.ToLower(c.SessionsBackend) {
	case sessionsCookieBackend:
		s.s, err = initCookieSession(c, infoFn, errFn)
	case sessionsFSBackend:
		fallthrough
	default:
		if strings.ToLower(c.SessionsBackend) != sessionsFSBackend {
			infoFn(log.Ctx{"backend": c.SessionsBackend})("Invalid session backend, falling back to %s.", sessionsFSBackend)
			c.SessionsBackend = sessionsFSBackend
		}
		s.s, err = initFileSession(c, infoFn, errFn)
	}
	return s, err
}

func hideSessionKeys(keys ...[]byte) []string {
	hidden := make([]string, len(keys))
	for i, k := range keys {
		hidden[i] = hideString(string(k))
	}
	return hidden
}

func initCookieSession(c appConfig, infoFn, errFn CtxLogFn) (sessions.Store, error) {
	ss := sessions.NewCookieStore(c.SessionKeys...)
	ss.Options.Path = "/"
	ss.Options.HttpOnly = true
	ss.Options.Secure = c.Secure
	ss.Options.SameSite = http.SameSiteLaxMode
	ss.Options.Domain = c.HostName

	infoFn(log.Ctx{
		"type":   c.SessionsBackend,
		"env":    c.Env,
		"keys":   hideSessionKeys(c.SessionKeys...),
		"domain": c.HostName,
	})("Session settings")
	if c.Env.IsProd() {
		ss.Options.SameSite = http.SameSiteStrictMode
	}
	return ss, nil
}

func initFileSession(c appConfig, infoFn, errFn CtxLogFn) (sessions.Store, error) {
	sessDir := fmt.Sprintf("%s/%s/%s", c.SessionsPath, c.Env, c.HostName)
	if _, err := os.Stat(sessDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(sessDir, 0700); err != nil {
				return nil, err
			}
		} else {
			errFn()("Invalid path %s for saving sessions: %s", sessDir, err)
			return nil, err
		}
	}
	infoFn(log.Ctx{
		"type":     c.SessionsBackend,
		"env":      c.Env,
		"path":     sessDir,
		"keys":     hideSessionKeys(c.SessionKeys...),
		"hostname": c.HostName,
	})("Session settings")
	ss := sessions.NewFilesystemStore(sessDir, c.SessionKeys...)
	ss.Options.Path = "/"
	ss.Options.HttpOnly = true
	ss.Options.Secure = c.Secure
	ss.Options.SameSite = http.SameSiteLaxMode
	if c.Env.IsProd() {
		ss.Options.Domain = c.HostName
		ss.Options.SameSite = http.SameSiteStrictMode
	}
	ss.MaxLength(1 << 20)
	return ss, nil
}

func (s *sess) get(w http.ResponseWriter, r *http.Request) (*sessions.Session, error) {
	if !s.enabled {
		return nil, nil
	}
	if s.s == nil {
		return nil, errors.Newf("invalid session")
	}
	ss, err := s.s.Get(r, sessionName)
	if err != nil {
		ss.Options.MaxAge = -1
		s.s.Save(r, w, ss)
		ss, err = s.s.New(r, sessionName)
	}
	return ss, err
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	if c, _ := r.Cookie(sessionName); c != nil {
		c.Value = ""
		c.MaxAge = -1
		http.SetCookie(w, c)
	}
}

func (s *sess) save(w http.ResponseWriter, r *http.Request) error {
	if !s.enabled || s.s == nil {
		clearSessionCookie(w, r)
		return nil
	}
	ss, err := s.s.Get(r, sessionName)
	if err != nil {
		clearSessionCookie(w, r)
		return nil
	}
	if ss != nil && len(ss.Values) == 0 {
		ss.Options.MaxAge = -1
	}
	if err := s.s.Save(r, w, ss); err != nil {
		clearSessionCookie(w, r)
	}
	return nil
}

func (s *sess) addFlashMessages(typ flashType, w http.ResponseWriter, r *http.Request, msgs ...string) {
	ss, _ := s.get(w, r)
	for _, msg := range msgs {
		n := flash{typ, msg}
		ss.AddFlash(n)
	}
}

func (s *sess) loadFlashMessages(w http.ResponseWriter, r *http.Request) (func() []flash, error) {
	var flashData []flash
	flashFn := func() []flash { return flashData }

	ss, err := s.get(w, r)
	if err != nil || ss == nil {
		return flashFn, nil
	}
	flashes := ss.Flashes()
	// setting the local flashData value
	for _, int := range flashes {
		if int == nil {
			continue
		}
		if f, ok := int.(flash); ok {
			flashData = append(flashData, f)
		}
	}
	err = ss.Save(r, w)
	return flashFn, err
}
