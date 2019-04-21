package frontend

import (
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/internal/log"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"net/http"
)

const SessionUserKey = "__current_acct"

type loginModel struct {
	Title   string
	Account app.Account
	OAuth   bool
}

// ShowLogin handles POST /login requests
func (h *handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	pw := r.PostFormValue("pw")
	handle := r.PostFormValue("handle")

	backUrl := "/oauth/authorize"
	a, err := db.Config.LoadAccount(app.Filters{LoadAccountsFilter: app.LoadAccountsFilter{Handle: []string{handle}}})
	if err != nil {
		h.logger.Error(err.Error())
		h.addFlashMessage(Error, r, "Login failed: wrong handle or password")
		h.Redirect(w, r, backUrl, http.StatusSeeOther)
		return
	}
	if a.Metadata == nil {
		h.logger.WithContext(log.Ctx{
			"handle": handle,
		}).Error("invalid account metadata")
		h.addFlashMessage(Error, r, "Login failed: wrong handle or password")
		h.Redirect(w, r, backUrl, http.StatusSeeOther)
		return
	}
	h.logger.WithContext(log.Ctx{
		"pw":   fmt.Sprintf("%s", a.Metadata.Password),
		"salt": fmt.Sprintf("%2x", a.Metadata.Salt),
	}).Debug("Loaded password")
	salt := a.Metadata.Salt
	saltyPw := []byte(pw)
	saltyPw = append(saltyPw, salt...)
	err = bcrypt.CompareHashAndPassword(a.Metadata.Password, saltyPw)

	if err != nil {
		h.logger.Error(err.Error())
		h.addFlashMessage(Error, r, "Login failed: wrong handle or password")
		h.Redirect(w, r, backUrl, http.StatusSeeOther)
		return
	}

	s, _ := h.sstor.Get(r, sessionName)
	s.Values[SessionUserKey] = sessionAccount{
		Handle: a.Handle,
		Hash:   []byte(a.Hash),
	}
	if err := s.Save(r, w); err != nil {
		h.logger.Error(err.Error())
	}

	config := GetOauth2Config("local", h.conf.BaseURL)
	h.Redirect(w, r, config.AuthCodeURL("state", oauth2.AccessTypeOnline), http.StatusPermanentRedirect)
}

// ShowLogin serves GET /login requests
func (h *handler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	a := app.Account{}

	m := loginModel{Title: "Login"}
	m.Account = a

	h.RenderTemplate(r, w, "login", m)
}

// HandleLogout serves /logout requests
func (h *handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	s, err := h.sstor.Get(r, sessionName)
	if err != nil {
		h.logger.Error(err.Error())
	}
	s.Values[SessionUserKey] = nil
	backUrl := "/"
	if r.Header.Get("Referer") != "" {
		backUrl = r.Header.Get("Referer")
	}
	h.Redirect(w, r, backUrl, http.StatusSeeOther)
}
