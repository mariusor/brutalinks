package app

import (
	"net/http"

	"encoding/json"
	log "github.com/sirupsen/logrus"

	"github.com/juju/errors"
	"github.com/mariusor/littr.go/models"
	"golang.org/x/crypto/bcrypt"
)

const SessionUserKey = "__current_acct"

type loginModel struct {
	Title         string
	InvertedTheme bool
	Account       models.Account
}

// ShowLogin handles POST /login requests
func HandleLogin(w http.ResponseWriter, r *http.Request) {

	errs := make([]error, 0)
	pw := r.PostFormValue("pw")
	handle := r.PostFormValue("handle")
	a, err := models.LoadAccount(Db, handle)
	if err != nil {
		log.Print(err)
		HandleError(w, r, StatusUnknown, errors.Errorf("handle or password are wrong"))
		return
	}
	m := &models.AccountMetadata{}
	err = json.Unmarshal(a.Metadata, m)
	if err != nil {
		log.Print(err)
		HandleError(w, r, StatusUnknown, errors.Errorf("handle or password are wrong"))
		return
	}
	log.Printf("Loaded pw: %q, salt: %q", m.Password, m.Salt)
	salt := m.Salt
	saltedpw := []byte(pw)
	saltedpw = append(saltedpw, salt...)
	err = bcrypt.CompareHashAndPassword(m.Password, saltedpw)
	if err != nil {
		log.Print(err)
		HandleError(w, r, StatusUnknown, errors.Errorf("handle or password are wrong"))
		return
	}

	s := GetSession(r)
	acct := Account{
		Id:        a.Id,
		Handle:    a.Handle,
		Email:     a.Email,
		Hash:      a.Hash(),
		Score:     a.Score,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
		flags:     a.Flags,
	}
	s.Values[SessionUserKey] = acct
	CurrentAccount = &acct
	AddFlashMessage(Success, "Login successful", r, w)

	err = SessionStore.Save(r, w, s)
	if err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		HandleError(w, r, http.StatusInternalServerError, errs...)
		return
	}
	Redirect(w, r, "/", http.StatusSeeOther)
	return
}

// ShowLogin serves GET /login requests
func ShowLogin(w http.ResponseWriter, r *http.Request) {
	a := models.Account{}

	m := loginModel{Title: "Login", InvertedTheme: IsInverted(r)}
	m.Account = a

	RenderTemplate(r, w, "login", m)
}

// HandleLogout serves /logout requests
func HandleLogout(w http.ResponseWriter, r *http.Request) {
	s := GetSession(r)
	s.Values[SessionUserKey] = nil
	SessionStore.Save(r, w, s)
	CurrentAccount = AnonymousAccount()
	Redirect(w, r, "/", http.StatusSeeOther)
}
