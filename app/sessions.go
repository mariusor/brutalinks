package app

import (
	"encoding/gob"
	"github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/gorilla/sessions"
	"github.com/mariusor/go-littr/internal/log"
	"net/http"
	"os"
	"path"
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
	path    string
	name    string
	s       sessions.Store
	infoFn  CtxLogFn
	errFn   CtxLogFn
}

func initSession(c appConfig, infoFn, errFn CtxLogFn) (sess, error) {
	// session encoding for account and flash message objects
	gob.Register(Account{})
	gob.Register(flash{})
	gob.Register(activitypub.Activity{})
	gob.Register(activitypub.IRI(""))
	gob.Register(activitypub.Object{})
	gob.Register(activitypub.ItemCollection{})
	gob.Register(activitypub.Link{})

	if len(c.SessionKeys) == 0 {
		return sess{}, errors.NotImplementedf("no session encryption keys, unable to use sessions")
	}
	s := sess{
		name:    sessionName,
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
		s.path = path.Join(c.SessionsPath, string(c.Env), c.HostName)
		s.s, err = initFileSession(c, s.path, infoFn, errFn)
	}
	if err != nil {
		s.enabled = false
	}
	return s, nil
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

func makeSessionsPath(path string) error {
	err := os.MkdirAll(path, 0700)
	if err != nil {
		return err
	}
	return nil
}

func initFileSession(c appConfig, path string, infoFn, errFn CtxLogFn) (sessions.Store, error) {
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		if err := makeSessionsPath(path); err != nil {
			return nil, err
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	f.Close()
	infoFn(log.Ctx{
		"type":     c.SessionsBackend,
		"env":      c.Env,
		"path":     path,
		"keys":     hideSessionKeys(c.SessionKeys...),
		"hostname": c.HostName,
	})("Session settings")
	ss := sessions.NewFilesystemStore(path, c.SessionKeys...)
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

func (s *sess) clear(w http.ResponseWriter, r *http.Request) error {
	if !s.enabled {
		return nil
	}
	if s.s == nil {
		return errors.Newf("invalid session")
	}
	ss, _ := s.s.Get(r, s.name)
	ss.Options.MaxAge = -1
	http.SetCookie(w, sessions.NewCookie(ss.Name(), "", ss.Options))
	return nil
}

func (s *sess) get(w http.ResponseWriter, r *http.Request) (*sessions.Session, error) {
	if !s.enabled {
		return nil, nil
	}
	if s.s == nil {
		return nil, errors.Newf("invalid session")
	}
	ss, err := s.s.Get(r, s.name)
	if os.IsNotExist(err) {
		err = nil
	}
	return ss, err
}

func (s *sess) save(w http.ResponseWriter, r *http.Request) error {
	if !s.enabled || s.s == nil {
		s.clear(w, r)
		return nil
	}
	ss, err := s.s.Get(r, s.name)
	if err != nil {
		s.clear(w, r)
	}
	if len(ss.Values) > 0 || len(ss.Flashes()) > 0 {
		return s.s.Save(r, w, ss)
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
	return flashFn, ss.Save(r, w)
}
