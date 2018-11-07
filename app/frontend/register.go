package frontend

import (
	"github.com/mariusor/littr.go/app"
	"net/http"
	"time"

	"html/template"

	"github.com/juju/errors"

	"github.com/gorilla/securecookie"
	"github.com/mariusor/littr.go/app/db"
	"golang.org/x/crypto/bcrypt"
)

type registerModel struct {
	Title         string
	InvertedTheme bool
	Terms         template.HTML
	Account       app.Account
}

func AccountFromRequest(r *http.Request) (*app.Account, []error) {
	if r.Method != http.MethodPost {
		return nil, []error{errors.Errorf("invalid http method type")}
	}
	errs := make([]error, 0)
	a := app.Account{}
	pw := r.PostFormValue("pw")
	pwConfirm := r.PostFormValue("pw-confirm")
	if pw != pwConfirm {
		errs = append(errs, errors.Errorf("the passwords don't match"))
	}

	/*
		agree := r.PostFormValue("agree")
		if agree != "y" {
			errs = append(errs, errors.Errorf("you must agree not to be a dick to other people"))
		}
	*/

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

	a.Hash = app.Hash(app.GenKey([]byte(a.Handle)).String())
	salt := securecookie.GenerateRandomKey(8)
	saltedpw := []byte(pw)
	saltedpw = append(saltedpw, salt...)

	savpw, err := bcrypt.GenerateFromPassword(saltedpw, 14)
	if err != nil {
		Logger.Error(err.Error())
	}
	a.Metadata = &app.AccountMetadata{
		Salt:     salt,
		Password: savpw,
	}

	a, err = db.Config.SaveAccount(a)
	Logger.Warn("using hardcoded db.Config.SaveAccount")
	if err != nil {
		Logger.Error(err.Error())
		return nil, []error{err}
	}
	return &a, nil
}

// ShowRegister serves GET /register requests
func ShowRegister(w http.ResponseWriter, r *http.Request) {
	m := registerModel{InvertedTheme: isInverted(r)}
	m.Terms = `<p>We try to follow <q><cite>Wheaton's Law</cite></q>:<br/>` +
		`<blockquote>Don't be a dick!</blockquote></p>`

	RenderTemplate(r, w, "register", m)
}

// HandleRegister handles POST /register requests
func HandleRegister(w http.ResponseWriter, r *http.Request) {
	a, errs := AccountFromRequest(r)

	if len(errs) > 0 {
		HandleError(w, r, http.StatusInternalServerError, errs...)
		return
	}
	Redirect(w, r, a.GetLink(), http.StatusSeeOther)
	return
}
