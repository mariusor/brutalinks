package frontend

import (
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/internal/log"
	"net/http"
	"time"

	"github.com/juju/errors"

	"github.com/gorilla/securecookie"
	"github.com/mariusor/littr.go/app/db"
	"golang.org/x/crypto/bcrypt"
)

type registerModel struct {
	Title         string
	Account       app.Account
}

func accountFromRequest(r *http.Request, l log.Logger) (*app.Account, []error) {
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
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now

	salt := securecookie.GenerateRandomKey(8)
	saltedpw := []byte(pw)
	saltedpw = append(saltedpw, salt...)

	savpw, err := bcrypt.GenerateFromPassword(saltedpw, 14)
	if err != nil {
		l.Error(err.Error())
	}
	a.Metadata = &app.AccountMetadata{
		Salt:     salt,
		Password: savpw,
	}

	a, err = db.Config.SaveAccount(a)
	l.Warn("using hardcoded db.Config.SaveAccount")
	if err != nil {
		l.Error(err.Error())
		return nil, []error{err}
	}
	return &a, nil
}

// ShowRegister serves GET /register requests
func (h *handler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	m := registerModel{}

	h.RenderTemplate(r, w, "register", m)
}

// HandleRegister handles POST /register requests
func (h *handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	a, errs := accountFromRequest(r, h.logger)

	if len(errs) > 0 {
		h.HandleError(w, r, errs...)
		return
	}
	h.Redirect(w, r, a.GetLink(), http.StatusSeeOther)
	return
}
