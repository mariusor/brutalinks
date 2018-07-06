package app

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"html/template"

	"github.com/gorilla/securecookie"
	"github.com/mariusor/littr.go/models"
	"golang.org/x/crypto/bcrypt"
)

type registerModel struct {
	Title         string
	InvertedTheme bool
	Terms         template.HTML
	Account       models.Account
}

func (l *Littr) AccountFromRequest(r *http.Request) (*models.Account, error) {
	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("invalid http method type")
	}
	a := models.Account{}
	pw := r.PostFormValue("pw")
	pwConfirm := r.PostFormValue("pw-confirm")
	if pw != pwConfirm {
		return nil, fmt.Errorf("passwords are not matching")
	}
	if pw != pwConfirm {
		return nil, fmt.Errorf("passwords are not matching")
	}

	agree := r.PostFormValue("agree")
	if agree != "y" {
		return nil, fmt.Errorf("you must agree not to be a dick to other people")
	}

	handle := r.PostFormValue("handle")
	if handle != "" {
		a.Handle = handle
	}
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now

	a.Key = a.GetKey()
	salt := securecookie.GenerateRandomKey(8)
	saltedpw := []byte(pw)
	saltedpw = append(saltedpw, salt...)

	savpw, err := bcrypt.GenerateFromPassword(saltedpw, 14)
	if err != nil {
		log.Print(err)
	}
	m := models.AccountMetadata{
		Salt:     salt,
		Password: savpw,
	}
	a.Metadata, err = json.Marshal(m)
	if err != nil {
		log.Print(err)
	}
	ins := `insert into "accounts" ("key", "handle", "created_at", "updated_at") values($1, $2, $3, $4)`
	{
		res, err := l.Db.Exec(ins, a.Key, a.Handle, a.CreatedAt, a.UpdatedAt)
		if err != nil {
			return nil, err
		} else {
			if rows, _ := res.RowsAffected(); rows == 0 {
				return nil, fmt.Errorf("could not save account %q", a.Hash())
			}
		}
	}

	return &a, nil
}
func (l *Littr) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		a, err := l.AccountFromRequest(r)

		if err != nil {
			l.HandleError(w, r, err, http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, a.PermaLink(), http.StatusMovedPermanently)
		return
	}

	m := registerModel{InvertedTheme: l.InvertedTheme}
	m.Terms = `<p>We try to follow <q><cite>Wheaton's Law</cite></q>:<br/>` +
		`<blockquote>Don't be a dick!</blockquote></p>`

	RenderTemplate(w, "register.html", m)
}
