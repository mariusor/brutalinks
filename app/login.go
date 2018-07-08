package app

import (
	"fmt"
	"net/http"

	"github.com/mariusor/littr.go/models"
)

const SessionUserKey = "acct"

type loginModel struct {
	Title         string
	InvertedTheme bool
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
			l.HandleError(w, r, StatusUnknown, err)
			return
		}
		for rows.Next() {
			err = rows.Scan(&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.UpdatedAt, &a.Metadata, &a.Flags)
			if err != nil {
				l.HandleError(w, r, StatusUnknown, err)
				return
			}
		}
		if pw == "" {
			errs = append(errs, fmt.Errorf("handle or password are wrong"))
		}

		s := l.GetSession(r)
		s.Values[SessionUserKey] = a
		s.AddFlash("Success")

		err = l.SessionStore.Save(r, w, l.GetSession(r))
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			l.HandleError(w, r, http.StatusInternalServerError, errs...)
			return
		}
		http.Redirect(w, r, a.PermaLink(), http.StatusMovedPermanently)
		return
	}

	q := r.URL.Query()
	logout := len(q["logout"]) > 0
	if logout {
		s := l.GetSession(r)
		s.Values[SessionUserKey] = nil
		l.SessionStore.Save(r, w, s)
		*CurrentAccount = models.AnonymousAccount()
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	}
	m := loginModel{InvertedTheme: l.InvertedTheme}
	//m.Account = a

	RenderTemplate(w, "login.html", m)
}