package frontend

import (
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/log"
	"net/http"

	"github.com/juju/errors"
	"golang.org/x/crypto/bcrypt"
)

const SessionUserKey = "__current_acct"

type loginModel struct {
	Title         string
	InvertedTheme bool
	Account       app.Account
}

// ShowLogin handles POST /login requests
func (h *handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	pw := r.PostFormValue("pw")
	handle := r.PostFormValue("handler")
	a, err := db.Config.LoadAccount(app.LoadAccountsFilter{Handle: []string{handle}})
	if err != nil {
		h.logger.Error(err.Error())
		h.HandleError(w, r, http.StatusForbidden, errors.Errorf("handler or password are wrong"))
		return
	}
	m := a.Metadata
	if m != nil {
		h.logger.WithContext(log.Ctx{
			"pw":   fmt.Sprintf("%2x", m.Password),
			"salt": fmt.Sprintf("%2x", m.Salt),
		}).Info("Loaded password")
		salt := m.Salt
		saltyPw := []byte(pw)
		saltyPw = append(saltyPw, salt...)
		err = bcrypt.CompareHashAndPassword(m.Password, saltyPw)
	} else {
		h.logger.Info(err.Error())
		h.HandleError(w, r, http.StatusForbidden, errors.Errorf("invalid account metadata"))
		return
	}
	if err != nil {
		h.logger.Error(err.Error())
		h.HandleError(w, r, http.StatusForbidden, errors.Errorf("handler or password are wrong"))
		return
	}

	if s, err := h.session.Get(r, sessionName); err == nil {
		s.Values[SessionUserKey] = sessionAccount{
			Handle: a.Handle,
			Hash:   []byte(a.Hash),
		}
		s.Save(r, w)
	} else {
		h.logger.Error(err.Error())
	}

	backUrl := "/"
	//addFlashMessage(Success, "Login successful", r)
	h.Redirect(w, r, backUrl, http.StatusSeeOther)
}

// ShowLogin serves GET /login requests
func (h *handler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	a := app.Account{}

	m := loginModel{Title: "Login", InvertedTheme: isInverted(r)}
	m.Account = a

	h.RenderTemplate(r, w, "login", m)
}

// HandleLogout serves /logout requests
func (h *handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if s, err := h.session.Get(r, sessionName); err != nil {
		h.logger.Error(err.Error())
	} else {
		s.Values[SessionUserKey] = nil
	}
	backUrl := "/"
	if r.Header.Get("Referer") != "" {
		backUrl = r.Header.Get("Referer")
	}
	h.Redirect(w, r, backUrl, http.StatusSeeOther)
}
