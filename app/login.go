package app

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"net/http"
)

const SessionUserKey = "__current_acct"

// ShowLogin handles POST /login requests
func (h *handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	pw := r.PostFormValue("pw")
	handle := r.PostFormValue("handle")
	state := r.PostFormValue("state")

	config := GetOauth2Config("fedbox", h.conf.BaseURL)
	// Try to load actor from handle
	acct, err := h.storage.LoadAccount(Filters{
		LoadAccountsFilter: LoadAccountsFilter{
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
		h.v.addFlashMessage(Error, r, fmt.Sprintf("Login failed: %s", err))
		h.v.Redirect(w, r, "/login", http.StatusSeeOther)
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
		h.v.addFlashMessage(Error, r, "Login failed: invalid username or password")
		h.v.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if tok == nil {
		h.logger.WithContext(logrus.Fields{
			"handle": handle,
			"client": config.ClientID,
			"state":  state,
		}).Errorf("nil token received")
		h.v.addFlashMessage(Error, r, "Login failed: wrong handle or password")
		h.v.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	acct.Metadata.OAuth.Provider = "fedbox"
	acct.Metadata.OAuth.Token = tok.AccessToken
	acct.Metadata.OAuth.TokenType = tok.TokenType
	acct.Metadata.OAuth.RefreshToken = tok.RefreshToken
	s, _ := h.v.s.get(r)
	s.Values[SessionUserKey] = acct
	h.v.Redirect(w, r, "/", http.StatusSeeOther)
}

// ShowLogin serves GET /login requests
func (h *handler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	a := account(r)

	m := loginModel{Title: "Login"}
	m.Account = *a

	h.v.RenderTemplate(r, w, "login", m)
}

// HandleLogout serves /logout requests
func (h *handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	s, err := h.v.s.get(r)
	if err != nil {
		h.logger.Error(err.Error())
	}
	s.Values[SessionUserKey] = nil
	backUrl := "/"
	if r.Header.Get("Referer") != "" {
		backUrl = r.Header.Get("Referer")
	}
	h.v.Redirect(w, r, backUrl, http.StatusSeeOther)
}
