package brutalinks

import (
	"encoding/gob"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"git.sr.ht/~mariusor/brutalinks/internal/config"
	log "git.sr.ht/~mariusor/lw"
	"git.sr.ht/~mariusor/mask"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/gorilla/sessions"
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
	gob.Register(vocab.Activity{})
	gob.Register(vocab.IRI(""))
	gob.Register(vocab.NaturalLanguageValues{})
	gob.Register(vocab.Object{})
	gob.Register(vocab.Actor{})
	gob.Register(vocab.ItemCollection{})
	gob.Register(vocab.Link{})
	gob.Register(vocab.Tombstone{})

	if len(c.SessionKeys) == 0 {
		c.SessionsEnabled = false
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
	case config.SessionsCookieBackend:
		s.s, err = initCookieSession(c, infoFn, errFn)
	case config.SessionsFSBackend:
		fallthrough
	default:
		if strings.ToLower(c.SessionsBackend) != config.SessionsFSBackend {
			infoFn(log.Ctx{"backend": c.SessionsBackend})("Invalid session backend, falling back to %s.", config.SessionsFSBackend)
			c.SessionsBackend = config.SessionsFSBackend
		}
		s.path = filepath.Clean(filepath.Join(c.SessionsPath, string(c.Env), c.HostName))
		s.s, err = initFileSession(c, s.path, infoFn, errFn)
	}
	if err != nil {
		s.enabled = false
	}
	return s, nil
}

func maskSessionKeys(keys ...[]byte) []string {
	hidden := make([]string, len(keys))
	for i, k := range keys {
		hidden[i] = mask.B(k).String()
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
		"keys":   maskSessionKeys(c.SessionKeys...),
		"domain": c.HostName,
	})("Session settings")
	if !c.Env.IsDev() {
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
	if _, err := os.Stat(path); err != nil && IsNotExist(err) {
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
		"keys":     maskSessionKeys(c.SessionKeys...),
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
	ss.MaxLength(1 << 24)
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
	if IsNotExist(err) {
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
		return ss.Save(r, w)
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
	flashFn := func() []flash {
		return flashData
	}

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
	// NOTE(marius): this last save is used to ensure the flash messages are removed between page loads
	// There should be a better way to achieve this.
	return flashFn, ss.Save(r, w)
}
