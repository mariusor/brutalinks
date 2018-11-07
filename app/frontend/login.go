package frontend

import (
	"github.com/mariusor/littr.go/app"
	"net/http"

	"github.com/mariusor/littr.go/app/db"

	log "github.com/inconshreveable/log15"

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
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	pw := r.PostFormValue("pw")
	handle := r.PostFormValue("handle")
	a, err := db.Config.LoadAccount(app.LoadAccountsFilter{Handle: []string{handle}})
	if err != nil {
		Logger.Error(err.Error())
		HandleError(w, r, http.StatusForbidden, errors.Errorf("handle or password are wrong"))
		return
	}
	m := a.Metadata
	if m != nil {
		Logger.Info("Loaded password", log.Ctx{"pw": string(m.Password), "salt": m.Salt})
		salt := m.Salt
		saltyPw := []byte(pw)
		saltyPw = append(saltyPw, salt...)
		err = bcrypt.CompareHashAndPassword(m.Password, saltyPw)
	} else {
		log.Info(err.Error())
		HandleError(w, r, http.StatusForbidden, errors.Errorf("invalid account metadata"))
		return
	}
	if err != nil {
		Logger.Error(err.Error())
		HandleError(w, r, http.StatusForbidden, errors.Errorf("handle or password are wrong"))
		return
	}

	if s, err := sessionStore.Get(r, sessionName); err == nil {
		s.Values[SessionUserKey] = sessionAccount{
			Handle: a.Handle,
			Hash:   []byte(a.Hash),
		}
		s.Save(r, w)
	} else {
		Logger.Error(err.Error())
	}

	backUrl := "/"
	//addFlashMessage(Success, "Login successful", r)
	Redirect(w, r, backUrl, http.StatusSeeOther)
}

// ShowLogin serves GET /login requests
func ShowLogin(w http.ResponseWriter, r *http.Request) {
	a := app.Account{}

	m := loginModel{Title: "Login", InvertedTheme: isInverted(r)}
	m.Account = a

	RenderTemplate(r, w, "login", m)
}

// HandleLogout serves /logout requests
func HandleLogout(w http.ResponseWriter, r *http.Request) {
	if s, err := sessionStore.Get(r, sessionName); err != nil {
		Logger.Error(err.Error())
	} else {
		s.Values[SessionUserKey] = nil
	}
	backUrl := "/"
	if r.Header.Get("Referer") != "" {
		backUrl = r.Header.Get("Referer")
	}
	Redirect(w, r, backUrl, http.StatusSeeOther)
}
