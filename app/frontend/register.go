package frontend

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/csrf"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/openshift/osin"
	"golang.org/x/oauth2"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-ap/errors"
)

type registerModel struct {
	Title   string
	Account app.Account
}

func accountFromRequest(r *http.Request, l log.Logger) (*app.Account, error) {
	if r.Method != http.MethodPost {
		return nil, errors.Errorf("invalid http method type")
	}

	a := app.Account{}
	pw := r.PostFormValue("pw")
	pwConfirm := r.PostFormValue("pw-confirm")
	if pw != pwConfirm {
		return nil, errors.Errorf("the passwords don't match")
	}

	/*
		agree := r.PostFormValue("agree")
		if agree != "y" {
			errs = append(errs, errors.Errorf("you must agree not to be a dick to other people"))
		}
	*/
	handle := r.PostFormValue("handle")
	if handle != "" {
		a.Handle = handle
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now

	a.Metadata = &app.AccountMetadata{
		Password: []byte(pw),
	}

	return &a, nil
}

// ShowRegister serves GET /register requests
func (h *handler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	m := registerModel{}

	h.RenderTemplate(r, w, "register", m)
}

var scopeAnonymousUserCreate = "anonUserCreate"

// HandleRegister handles POST /register requests
func (h *handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	a, err := accountFromRequest(r, h.logger)
	if err != nil {
		h.HandleErrors(w, r, err)
		return
	}

	maybeExists, err := h.storage.LoadAccount(app.Filters{
		LoadAccountsFilter: app.LoadAccountsFilter{
			Handle: []string{a.Handle},
		},
	})
	notFound := errors.NotFoundf("")
	if err != nil && !notFound.As(err) {
		h.HandleErrors(w, r, errors.NewBadRequest(err, "unable to create"))
		return
	}
	if maybeExists.IsValid() {
		h.HandleErrors(w, r, errors.BadRequestf("account %s already exists", a.Handle))
		return
	}

	*a, err = h.storage.SaveAccount(*a)
	if err != nil {
		h.HandleErrors(w, r, err)
		return
	}
	if !a.IsValid() || !a.HasMetadata() || a.Metadata.ID == "" {
		h.HandleErrors(w, r, errors.Newf("unable to save actor"))
		return
	}

	// TODO(marius): Start oauth2 authorize session
	config := GetOauth2Config("fedbox", h.conf.BaseURL)
	config.Scopes = []string{scopeAnonymousUserCreate}
	param := oauth2.SetAuthURLParam("actor", a.Metadata.ID)
	sessUrl := config.AuthCodeURL(csrf.Token(r), param)

	res, err := http.Get(sessUrl)
	if err != nil {
		h.HandleErrors(w, r, err)
		return
	}

	var body []byte
	if body, err = ioutil.ReadAll(res.Body); err != nil {
		h.HandleErrors(w, r, err)
		return
	}
	d := osin.AuthorizeData{}
	if err := json.Unmarshal(body, &d); err != nil {
		h.HandleErrors(w, r, err)
		return
	}

	if d.Code == "" {
		h.HandleErrors(w, r, errors.NotValidf("unable to get session token for setting the user's password"))
		return
	}

	// pos
	pwChURL := fmt.Sprintf("%s/oauth/pw", h.storage.BaseURL)
	u, _ := url.Parse(pwChURL)
	q := u.Query()
	q.Set("s", d.Code)
	u.RawQuery = q.Encode()

	form := url.Values{}
	pw := r.PostFormValue("pw")
	pwConfirm := r.PostFormValue("pw-confirm")

	form.Add("pw", pw)
	form.Add("pw-confirm", pwConfirm)

	pwChRes, err := http.Post(u.String(), "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if body, err = ioutil.ReadAll(pwChRes.Body); err != nil {
		h.logger.Error(err.Error())
		h.HandleErrors(w, r, err)
		return
	}
	if pwChRes.StatusCode != http.StatusOK {
		h.HandleErrors(w, r, h.storage.handlerErrorResponse(body))
		return
	}
	h.Redirect(w, r, "/", http.StatusSeeOther)
	return
}
