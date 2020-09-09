package app

import (
	"github.com/go-ap/errors"
	"github.com/gorilla/sessions"
	"net/http"
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

func (s *session) get(w http.ResponseWriter, r *http.Request) (*sessions.Session, error) {
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

func (s *session) save(w http.ResponseWriter, r *http.Request) error {
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

func (s *session) addFlashMessages(typ flashType, w http.ResponseWriter, r *http.Request, msgs ...string) {
	ss, _ := s.get(w, r)
	for _, msg := range msgs {
		n := flash{typ, msg}
		ss.AddFlash(n)
	}
}

func (s *session) loadFlashMessages(w http.ResponseWriter, r *http.Request) (func() []flash, error) {
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