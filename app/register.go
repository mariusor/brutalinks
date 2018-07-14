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
	InvertedTheme func(r *http.Request) bool
	Terms         template.HTML
	Account       models.Account
}

func AccountFromRequest(r *http.Request) (*models.Account, []error) {
	if r.Method != http.MethodPost {
		return nil, []error{fmt.Errorf("invalid http method type")}
	}
	errs := make([]error, 0)
	a := models.Account{}
	pw := r.PostFormValue("pw")
	pwConfirm := r.PostFormValue("pw-confirm")
	if pw != pwConfirm {
		errs = append(errs, fmt.Errorf("the passwords don't match"))
	}

	agree := r.PostFormValue("agree")
	if agree != "y" {
		errs = append(errs, fmt.Errorf("you must agree not to be a dick to other people"))
	}

	if len(errs) > 0 {
		return nil, errs
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
	ins := `insert into "accounts" ("key", "handle", "created_at", "updated_at", "metadata") values($1, $2, $3, $4, $5)`
	{
		res, err := Db.Exec(ins, a.Key, a.Handle, a.CreatedAt, a.UpdatedAt, a.Metadata)
		if err != nil {
			return nil, []error{err}
		} else {
			if rows, _ := res.RowsAffected(); rows == 0 {
				return nil, []error{fmt.Errorf("could not save account %q", a.Hash())}
			}
		}
	}

	return &a, nil
}

func HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		a, errs := AccountFromRequest(r)

		if len(errs) > 0 {
			HandleError(w, r, http.StatusInternalServerError, errs...)
			return
		}
		http.Redirect(w, r, a.GetLink(), http.StatusMovedPermanently)
		return
	}

	m := registerModel{InvertedTheme: IsInverted}
	m.Terms = `<p>We try to follow <q><cite>Wheaton's Law</cite></q>:<br/>` +
		`<blockquote>Don't be a dick!</blockquote></p>`

	RenderTemplate(r, w, "register.html", m)
}
