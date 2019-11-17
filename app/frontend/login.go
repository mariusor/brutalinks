package frontend

import (
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/sirupsen/logrus"
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
	state := r.PostFormValue("state")

	config := GetOauth2Config("fedbox", h.conf.BaseURL)
	// Try to load actor from handle
	acct, err := h.storage.LoadAccount(app.Filters{
		LoadAccountsFilter: app.LoadAccountsFilter{
			Handle:  []string{handle,},
			Deleted: []bool{false,},
		},
	})
	if err != nil {
		h.logger.WithContext(logrus.Fields{
			"handle": handle,
			"client": config.ClientID,
			"state":  state,
		}).Error(err.Error())
		h.addFlashMessage(Error, r, fmt.Sprintf("Login failed: %s", err))
		h.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tok, err := config.PasswordCredentialsToken(r.Context(), handle, pw)
	if err != nil {
		h.logger.WithContext(logrus.Fields{
			"handle": handle,
			"client": config.ClientID,
			"state":  state,
			"error": err,
		}).Error("login failed")
		h.addFlashMessage(Error, r, "Login failed: invalid username or password")
		h.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if tok == nil {
		h.logger.WithContext(logrus.Fields{
			"handle": handle,
			"client": config.ClientID,
			"state":  state,
		}).Errorf("nil token received")
		h.addFlashMessage(Error, r, "Login failed: wrong handle or password")
		h.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	acct.Metadata.OAuth.Provider = "fedbox"
	acct.Metadata.OAuth.Token = tok.AccessToken
	acct.Metadata.OAuth.TokenType = tok.TokenType
	acct.Metadata.OAuth.RefreshToken = tok.RefreshToken
	s, _ := h.sstor.Get(r, sessionName)
	s.Values[SessionUserKey] = acct
	h.Redirect(w, r, "/", http.StatusSeeOther)
}

// ShowLogin serves GET /login requests
func (h *handler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	a := h.account(r)

	m := loginModel{Title: "Login"}
	m.Account = *a

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
