package app

import (
	"fmt"
	"net/http"

	"encoding/json"
	"log"

	"github.com/mariusor/littr.go/models"
	"golang.org/x/crypto/bcrypt"
)

const SessionUserKey = "acct"

type loginModel struct {
	Title         string
	InvertedTheme func(r *http.Request) bool
	Account       models.Account
}

func (l *Littr) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		errs := make([]error, 0)
		a := models.Account{}
		pw := r.PostFormValue("pw")
		handle := r.PostFormValue("handle")
		sel := `select "id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags" from "accounts" where "handle" = $1`
		rows, err := l.Db.Query(sel, handle)
		if err != nil {
			HandleError(w, r, StatusUnknown, err)
			return
		}
		for rows.Next() {
			err = rows.Scan(&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.UpdatedAt, &a.Metadata, &a.Flags)
			if err != nil {
				HandleError(w, r, StatusUnknown, err)
				return
			}
		}
		m := &models.AccountMetadata{}
		err = json.Unmarshal(a.Metadata, m)
		if err != nil {
			log.Print(err)
			HandleError(w, r, StatusUnknown, fmt.Errorf("handle or password are wrong"))
			return
		}
		salt := m.Salt
		saltedpw := []byte(pw)
		saltedpw = append(saltedpw, salt...)
		err = bcrypt.CompareHashAndPassword(m.Password, saltedpw)
		if err != nil {
			log.Print(err)
			HandleError(w, r, StatusUnknown, fmt.Errorf("handle or password are wrong"))
			return
		}

		s := l.GetSession(r)
		s.Values[SessionUserKey] = a
		s.AddFlash("Success")

		err = l.SessionStore.Save(r, w, l.GetSession(r))
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			HandleError(w, r, http.StatusInternalServerError, errs...)
			return
		}
		http.Redirect(w, r, a.GetLink(), http.StatusMovedPermanently)
		return
	}

	q := r.URL.Query()
	logout := len(q["logout"]) > 0
	if logout {
		s := l.GetSession(r)
		s.Values[SessionUserKey] = nil
		l.SessionStore.Save(r, w, s)
		CurrentAccount = AnonymousAccount()
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	}
	m := loginModel{InvertedTheme: IsInverted}
	//m.Account = a

	RenderTemplate(w, "login.html", m)
}
