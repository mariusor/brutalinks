package oauth

import (
	"database/sql"
	"fmt"
	"github.com/go-chi/chi"
	_ "github.com/lib/pq"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/openshift/osin"
	"net/http"
)

type handler struct {
	s *osin.Server
}

type Config struct {
	DB struct {
		Enabled bool
		Host    string
		Port    string
		User    string
		Pw      string
		Name    string
	}
	Logger log.Logger
}

type logger struct {
	l log.Logger
}

func (l logger) Printf(format string, v ...interface{}) {
	l.l.Infof(format, v...)
}

func Init(c Config) handler {
	config := osin.ServerConfig{
		AuthorizationExpiration:   250,
		AccessExpiration:          3600,
		TokenType:                 "Bearer",
		AllowedAuthorizeTypes:     osin.AllowedAuthorizeType{osin.CODE},
		AllowedAccessTypes:        osin.AllowedAccessType{osin.AUTHORIZATION_CODE},
		ErrorStatusCode:           200,
		AllowClientSecretInParams: false,
		AllowGetAccessRequest:     false,
		RetainTokenAfterRefresh:   false,
	}
	url := fmt.Sprintf("postgres://%s/%s", c.DB.Host, c.DB.Name)
	db, err := sql.Open("postgres", url)
	h := handler{}
	if err == nil {
		store := New(db, c.Logger)
		h.s = osin.NewServer(&config, store)
	}
	h.s.Logger = logger{l: c.Logger}

	return h
}

func (h handler)Authorize(w http.ResponseWriter, r *http.Request) {
	resp := h.s.NewResponse()
	defer resp.Close()

	if ar := h.s.HandleAuthorizeRequest(resp, r); ar != nil {

		// HANDLE LOGIN PAGE HERE

		ar.Authorized = true
		h.s.FinishAuthorizeRequest(resp, r, ar)
	}
	osin.OutputJSON(resp, w, r)
}

func (h handler) Token(w http.ResponseWriter, r *http.Request) {
	resp := h.s.NewResponse()
	defer resp.Close()

	if ar := h.s.HandleAccessRequest(resp, r); ar != nil {
		ar.Authorized = true
		h.s.FinishAccessRequest(resp, r, ar)
	}
	osin.OutputJSON(resp, w, r)
}

func (h handler) Routes() func(chi.Router) {
	return func(r chi.Router) {
		// Authorization code endpoint
		r.Get("/authorize", h.Authorize)
		// Access token endpoint
		r.Get("/token", h.Token)
	}
}
