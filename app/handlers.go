package app

import (
	"context"
	"encoding/json"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/gorilla/csrf"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/mariusor/qstring"
	"github.com/openshift/osin"
	"golang.org/x/oauth2"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/go-chi/chi"
)

// HandleSubmit handles POST /submit requests
// HandleSubmit handles POST /~handler/hash requests
// HandleSubmit handles POST /year/month/day/hash requests
// HandleSubmit handles POST /~handler/hash/edit requests
// HandleSubmit handles POST /year/month/day/hash/edit requests
func (h *handler) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)
	ctx := context.Background()
	n, err := ContentFromRequest(r, *acc)
	if err != nil {
		h.errFn(log.Ctx{
			"before": err,
		})("Error: wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	saveVote := true

	repo := h.storage
	if n.Parent.IsValid() {
		c := ContextCursor(r.Context())
		if len(c.items) > 0 {
			n.Parent = getFromList(n.Parent.Hash, c.items)
		}
		if len(n.Metadata.To) == 0 {
			n.Metadata.To = make([]*Account, 0)
		}
		n.Metadata.To = append(n.Metadata.To, n.Parent.SubmittedBy)
		if n.Parent.Private() {
			n.MakePrivate()
			saveVote = false
		}
		if n.Parent.OP.IsValid() {
			n.OP = n.Parent.OP
		}
	}

	if len(n.Hash) > 0 {
		var iri pub.IRI
		if n.HasMetadata() && len(n.Metadata.ID) > 0 {
			iri = pub.IRI(n.Metadata.ID)
		} else {
			iri = ObjectsURL.AddPath(n.Hash.String())
		}
		if p, err := repo.LoadItem(ctx, iri); err == nil {
			n.Title = p.Title
		}
		saveVote = false
	}
	n, err = repo.SaveItem(ctx, n)
	if err != nil {
		h.errFn(log.Ctx{
			"before": err,
		})("Error: unable to save item")
		h.v.HandleErrors(w, r, err)
		return
	}

	if saveVote {
		v := Vote{
			SubmittedBy: acc,
			Item:        &n,
			Weight:      1 * ScoreMultiplier,
		}
		if _, err := repo.SaveVote(ctx, v); err != nil {
			h.errFn(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
			})("Error: %s", err)
		}
	}
	h.v.Redirect(w, r, ItemPermaLink(&n), http.StatusSeeOther)
}

func genitive(name string) string {
	l := len(name)
	if l == 0 {
		return name
	}
	if name[l-1:l] != "s" {
		return name + "'s"
	}
	return name + "'"
}

// HandleDelete serves /{year}/{month}/{day}/{hash}/rm POST request
// HandleDelete serves /~{handle}/rm GET request
func (h *handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	repo := h.storage
	iri := ObjectsURL.AddPath(chi.URLParam(r, "hash"))
	ctx := context.Background()
	p, err := repo.LoadItem(ctx, iri)
	if err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}

	url := ItemPermaLink(&p)
	backUrl := r.Header.Get("Referer")
	if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL) {
		url = fmt.Sprintf("%s#li-%s", backUrl, p.Hash)
	}
	p.Delete()
	if p, err = repo.SaveItem(ctx, p); err != nil {
		h.v.addFlashMessage(Error, w, r, "unable to delete item as current user")
	}

	h.v.Redirect(w, r, url, http.StatusFound)
}

// HandleVoting serves /{year}/{month}/{day}/{hash}/{direction} request
// HandleVoting serves /~{handle}/{direction} request
func (h *handler) HandleVoting(w http.ResponseWriter, r *http.Request) {
	repo := h.storage
	ctx := context.Background()
	iri := ObjectsURL.AddPath(chi.URLParam(r, "hash"))
	p, err := repo.LoadItem(ctx, iri)
	if err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}

	direction := path.Base(r.URL.Path)
	multiplier := 0
	switch strings.ToLower(direction) {
	case Yay:
		multiplier = 1
	case Nay:
		multiplier = -1
	}
	url := ItemPermaLink(&p)

	acc := loggedAccount(r)
	if acc.IsLogged() {
		backUrl := r.Header.Get("Referer")
		if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL) {
			url = fmt.Sprintf("%s#li-%s", backUrl, p.Hash)
		}
		v := Vote{
			SubmittedBy: acc,
			Item:        &p,
			Weight:      multiplier * ScoreMultiplier,
		}
		if _, err := repo.SaveVote(ctx, v); err != nil {
			h.errFn(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
				"error":  err,
			})("Error: Unable to save vote")
			h.v.addFlashMessage(Error, w, r, "Unable to save vote")
		}
	} else {
		h.v.addFlashMessage(Error, w, r, "unable to vote as current user")
	}
	h.v.Redirect(w, r, url, http.StatusFound)
}

func (h *handler) FollowAccount(w http.ResponseWriter, r *http.Request) {
	loggedAccount := loggedAccount(r)
	if !loggedAccount.IsValid() {
		err := errors.Unauthorizedf("invalid logged account")
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, err)
		return
	}
	repo := h.storage
	var err error
	toFollow := ContextAuthors(r.Context())
	if len(toFollow) == 0 {
		h.v.HandleErrors(w, r, errors.NotFoundf("account not found"))
		return
	}
	fol := toFollow[0]
	// todo(marius): load follow reason from POST request so we can show it to the followed user
	err = repo.FollowAccount(context.Background(), *loggedAccount, fol, nil)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	h.v.Redirect(w, r, AccountPermaLink(&fol), http.StatusSeeOther)
}

func (h *handler) HandleFollowRequest(w http.ResponseWriter, r *http.Request) {
	loggedAccount := loggedAccount(r)
	if !loggedAccount.IsValid() {
		err := errors.Unauthorizedf("invalid logged account")
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, err)
		return
	}

	ctx := context.Background()
	repo := h.storage
	followers := ContextAuthors(r.Context())
	if len(followers) == 0 {
		h.v.HandleErrors(w, r, errors.NotFoundf("account not found"))
		return
	}
	accept := false
	action := chi.URLParam(r, "action")
	if action == "accept" {
		accept = true
	}

	follower := followers[0]
	ff := &Filters{
		Actor: &Filters{
			IRI: CompStrs{LikeString(follower.Hash.String())},
		},
		Object: &Filters{
			IRI: CompStrs{LikeString(loggedAccount.Hash.String())},
		},
	}
	// todo(marius): load response reason from POST request so we can show it to the followed user
	followRequests, cnt, err := repo.LoadFollowRequests(ctx, loggedAccount, ff)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	if cnt == 0 {
		h.v.HandleErrors(w, r, errors.NotFoundf("follow request not found"))
		return
	}
	follow := followRequests[0]
	err = repo.SendFollowResponse(ctx, follow, accept, nil)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	backUrl := r.Header.Get("Referer")
	h.v.Redirect(w, r, backUrl, http.StatusSeeOther)
}

const SessionUserKey = "__current_acct"

// ShowLogin handles POST /login requests
func (h *handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	pw := r.PostFormValue("pw")
	handle := r.PostFormValue("handle")
	state := r.PostFormValue("state")
	ctx := context.Background()

	config := GetOauth2Config("fedbox", h.conf.BaseURL)
	// Try to load actor from handle
	accts, err := h.storage.accounts(ctx, &Filters{
		Name: CompStrs{EqualsString(handle)},
		Type: ActivityTypesFilter(ValidActorTypes...),
	})

	handleErr := func(msg string, f log.Ctx) {
		h.errFn(f)("Error: %s", err)
		h.v.addFlashMessage(Error, w, r, msg)
		h.v.Redirect(w, r, "/login", http.StatusSeeOther)
	}
	if err != nil || len(accts) == 0 {
		if err == nil {
			err = errors.NotFoundf("%s", handle)
		}
		handleErr(fmt.Sprintf("Login failed: %s", err), log.Ctx{
			"handle": handle,
			"client": config.ClientID,
			"state":  state,
			"err":    err,
		})
		return
	}
	acct := accts[0]

	tok, err := config.PasswordCredentialsToken(context.Background(), handle, pw)
	if err != nil || tok == nil {
		if err == nil {
			err = errors.Errorf("nil token received")
		}
		handleErr("Login failed: invalid username or password", log.Ctx{
			"handle": handle,
			"client": config.ClientID,
			"state":  state,
			"error":  fmt.Sprintf("%s", err),
		})
		return
	}
	acct.Metadata.OAuth.Provider = "fedbox"
	acct.Metadata.OAuth.Token = tok.AccessToken
	acct.Metadata.OAuth.TokenType = tok.TokenType
	acct.Metadata.OAuth.RefreshToken = tok.RefreshToken
	s, err := h.v.s.get(w, r)
	if err != nil {
		handleErr("Login failed: unable to save session", log.Ctx{
			"handle": handle,
			"client": config.ClientID,
			"state":  state,
			"error":  fmt.Sprintf("%s", err),
		})
		return
	}
	s.Values[SessionUserKey] = acct
	h.v.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleLogout serves /logout requests
func (h *handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	s, err := h.v.s.get(w, r)
	if err != nil {
		h.errFn()("Error: %s", err)
	} else {
		s.Values = nil
	}
	backUrl := "/"
	if r.Header.Get("Referer") != "" {
		backUrl = r.Header.Get("Referer")
	}
	h.v.Redirect(w, r, backUrl, http.StatusSeeOther)
}

func (h *handler) ValidatePermissions(actions ...string) func(http.Handler) http.Handler {
	if len(actions) == 0 {
		return h.ValidateItemAuthor
	}
	// @todo(marius): implement permission logic
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

func (h *handler) RedirectToLogin(w http.ResponseWriter, r *http.Request, errs ...error) {
	h.v.Redirect(w, r, "/login", http.StatusMovedPermanently)
}

func (h *handler) ValidateLoggedIn(eh ErrorHandler) Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			if !loggedAccount(r).IsLogged() {
				e := errors.Unauthorizedf("Please login to perform this action")
				h.errFn()("Error: %s", e)
				eh(w, r, e)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

func (h *handler) ValidateItemAuthor(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		acc := loggedAccount(r)
		hash := chi.URLParam(r, "hash")
		url := r.URL
		action := path.Base(url.Path)
		if len(hash) > 0 && action != hash {
			repo := h.storage
			iri := ObjectsURL.AddPath(hash)
			m, err := repo.LoadItem(ctx, iri)
			if err != nil {
				ctxtErr(next, w, r, errors.NewNotFound(err, "item"))
				return
			}
			if !HashesEqual(m.SubmittedBy.Hash, acc.Hash) {
				url.Path = path.Dir(url.Path)
				h.v.Redirect(w, r, url.RequestURI(), http.StatusTemporaryRedirect)
				return
			}
			next.ServeHTTP(w, r)
		}
	}
	return http.HandlerFunc(fn)
}

// HandleItemRedirect serves /i/{hash} request
func (h *handler) HandleItemRedirect(w http.ResponseWriter, r *http.Request) {
	repo := h.storage
	iri := ObjectsURL.AddPath(chi.URLParam(r, "hash"))
	ctx := context.Background()
	p, err := repo.LoadItem(ctx, iri)
	if err != nil {
		h.v.HandleErrors(w, r, errors.NewNotValid(err, "oops!"))
		return
	}
	url := ItemPermaLink(&p)
	h.v.Redirect(w, r, url, http.StatusMovedPermanently)
}

var scopeAnonymousUserCreate = "anonUserCreate"

func getPassCode(h *handler, acc *Account, invitee *Account, r *http.Request) (string, error) {
	if !acc.IsLogged() {
		acc = h.storage.app
	}

	// TODO(marius): Start oauth2 authorize session
	config := GetOauth2Config("fedbox", h.conf.BaseURL)
	config.Scopes = []string{scopeAnonymousUserCreate}
	param := oauth2.SetAuthURLParam("actor", invitee.pub.GetLink().String())
	sessUrl := config.AuthCodeURL(csrf.Token(r), param)
	res, err := http.Get(sessUrl)
	if err != nil {
		return "", err
	}

	var body []byte
	if body, err = ioutil.ReadAll(res.Body); err != nil {
		return "", err
	}
	d := osin.AuthorizeData{}
	if err := json.Unmarshal(body, &d); err != nil {
		return "", err
	}

	if d.Code == "" {
		return "", errors.NotValidf("unable to get session token for setting the user's password")
	}
	return d.Code, nil
}

// HandleSendInvite handles POST /invite requests
func (h *handler) HandleSendInvite(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)
	email := r.PostFormValue("email")
	if len(email) == 0 {
		h.v.HandleErrors(w, r, errors.BadRequestf("invalid email"))
		return
	}

	invitee, err := h.storage.SaveAccount(context.Background(), Account{
		Email:     email,
		CreatedBy: acc,
	})

	if err != nil {
		h.v.HandleErrors(w, r, errors.NewBadRequest(err, "unable to save account"))
		return
	}
	if !invitee.IsValid() || !invitee.HasMetadata() || invitee.Metadata.ID == "" {
		h.v.HandleErrors(w, r, errors.NewBadRequest(err, "invalid account saved"))
		return
	}

	// @todo(marius): :link_generation:
	u := fmt.Sprintf("%s/register/%s", h.conf.BaseURL, invitee.Hash)
	bodyFmt := "Hello %s,\nThis is an invitation to join %s: %s.\nTo accept this invitation and create an account, visit the URL below: %s\n/%s"
	mailContent := struct {
		Subject string `qstring:subject`
		Body    string `qstring:body`
	}{
		Subject: fmt.Sprintf("You are invited to join %s", h.conf.HostName),
		Body:    fmt.Sprintf(bodyFmt, invitee.Email, h.conf.HostName, h.conf.BaseURL, u, acc.Handle),
	}
	q, _ := qstring.Marshal(&mailContent)
	h.v.Redirect(w, r, fmt.Sprintf("mailto:%s?%s", invitee.Email, q), http.StatusSeeOther)
	return
}

// HandleRegister handles POST /register requests
func (h *handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	a, err := accountFromPost(r)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	ctx := context.Background()

	f := &Filters{Name: CompStrs{EqualsString(a.Handle)}}
	maybeExists, err := h.storage.fedbox.Actors(ctx, Values(f))
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	if maybeExists.Count() > 0 {
		h.v.HandleErrors(w, r, errors.BadRequestf("account %s already exists", a.Handle))
		return
	}

	app := h.storage.app
	a.CreatedBy = app
	a, err = h.storage.WithAccount(app).SaveAccount(ctx, a)
	if err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, err)
		return
	}
	if !a.IsValid() || !a.HasMetadata() || a.Metadata.ID == "" {
		h.v.HandleErrors(w, r, errors.Newf("unable to save actor"))
		return
	}

	// TODO(marius): Start oauth2 authorize session
	config := GetOauth2Config("fedbox", h.conf.BaseURL)
	config.Scopes = []string{scopeAnonymousUserCreate}
	param := oauth2.SetAuthURLParam("actor", a.Metadata.ID)
	sessUrl := config.AuthCodeURL(csrf.Token(r), param)

	res, err := http.Get(sessUrl)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}

	var body []byte
	if body, err = ioutil.ReadAll(res.Body); err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	if res.StatusCode != http.StatusOK {
		incoming, e := errors.UnmarshalJSON(body)
		var errs []error
		if e == nil {
			errs = make([]error, len(incoming))
			for i := range incoming {
				errs[i] = incoming[i]
			}
		} else {
			errs = []error{errors.WrapWithStatus(res.StatusCode, errors.Newf(""), "invalid response")}
		}
		h.v.HandleErrors(w, r, errs...)
		return
	}
	d := osin.AuthorizeData{}
	if err := json.Unmarshal(body, &d); err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	if d.Code == "" {
		h.v.HandleErrors(w, r, errors.NotValidf("unable to get session token for setting the user's password"))
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
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, err)
		return
	}
	if pwChRes.StatusCode != http.StatusOK {
		h.v.HandleErrors(w, r, h.storage.handlerErrorResponse(body))
		return
	}
	h.v.Redirect(w, r, "/", http.StatusSeeOther)
	return
}

// HandleShow serves most of the GET requests
func (h *handler) HandleShow(w http.ResponseWriter, r *http.Request) {
	m := ContextModel(r.Context())
	if m == nil {
		m = &errorModel{
			Status:     404,
			StatusText: "Oops!!",
			Title:      "Oops!!",
		}
	}
	cursor := ContextCursor(r.Context())
	if mod, ok := m.(Paginator); ok && cursor != nil {
		mod.SetCursor(cursor)
	}
	if err := h.v.RenderTemplate(r, w, m.Template(), m); err != nil {
		h.v.HandleErrors(w, r, err)
	}
}

// BlockAccount processes a report request received at /~{handle}/block
func (h *handler) BlockAccount(w http.ResponseWriter, r *http.Request) {
	loggedAccount := loggedAccount(r)
	if !loggedAccount.IsValid() {
		err := errors.Unauthorizedf("invalid logged account")
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, err)
		return
	}

	reason, err := ContentFromRequest(r, *loggedAccount)
	if err != nil {
		h.errFn(log.Ctx{
			"before": err,
		})("wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	repo := h.storage

	toBlock := ContextAuthors(r.Context())
	if len(toBlock) == 0 {
		h.v.HandleErrors(w, r, errors.NotFoundf("account not found"))
		return
	}
	block := toBlock[0]
	err = repo.BlockAccount(context.Background(), *loggedAccount, block, &reason)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	h.v.Redirect(w, r, PermaLink(&block), http.StatusSeeOther)
}

// BlockItem processes a block request received at /~{handle}/{hash}/block
func (h *handler) BlockItem(w http.ResponseWriter, r *http.Request) {
	loggedAccount := loggedAccount(r)
	if !loggedAccount.IsValid() {
		err := errors.Unauthorizedf("invalid logged account")
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, err)
		return
	}

	reason, err := ContentFromRequest(r, *loggedAccount)
	if err != nil {
		h.errFn(log.Ctx{
			"before": err,
		})("wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	repo := h.storage
	ctx := context.Background()

	it, err := repo.LoadItem(ctx, ObjectsURL.AddPath(chi.URLParam(r, "hash")))
	if err != nil {
		h.errFn(log.Ctx{
			"before": err,
		})("invalid item to report")
		h.v.HandleErrors(w, r, errors.NewNotFound(err, ""))
	}
	err = repo.BlockItem(ctx, *loggedAccount, it, &reason)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	h.v.Redirect(w, r, PermaLink(&it), http.StatusSeeOther)
}

// ReportAccount processes a report request received at /~{handle}/block
func (h *handler) ReportAccount(w http.ResponseWriter, r *http.Request) {
	loggedAccount := loggedAccount(r)
	if !loggedAccount.IsValid() {
		err := errors.Unauthorizedf("invalid logged account")
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, err)
		return
	}

	reason, err := ContentFromRequest(r, *loggedAccount)
	if err != nil {
		h.errFn(log.Ctx{
			"before": err,
		})("Error: wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	repo := h.storage

	byHandleAccounts := ContextAuthors(r.Context())
	if len(byHandleAccounts) == 0 {
		h.v.HandleErrors(w, r, errors.NotFoundf("account not found"))
		return
	}
	p := byHandleAccounts[0]
	err = repo.ReportAccount(context.Background(), *loggedAccount, p, &reason)
	if err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}
	url := AccountPermaLink(&p)

	backUrl := r.Header.Get("Referer")
	if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL) {
		url = fmt.Sprintf("%s#li-%s", backUrl, p.Hash)
	}
	h.v.Redirect(w, r, url, http.StatusFound)
}

// ReportItem processes a report request received at /~{handle}/{hash}/bad
func (h *handler) ReportItem(w http.ResponseWriter, r *http.Request) {
	loggedAccount := loggedAccount(r)
	if !loggedAccount.IsValid() {
		err := errors.Unauthorizedf("invalid logged account")
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, err)
		return
	}
	ctx := context.Background()

	reason, err := ContentFromRequest(r, *loggedAccount)
	if err != nil {
		h.errFn(log.Ctx{
			"before": err,
		})("Error: wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	repo := h.storage

	p, err := repo.LoadItem(ctx, ObjectsURL.AddPath(chi.URLParam(r, "hash")))
	if err != nil {
		h.errFn(log.Ctx{
			"before": err,
		})("invalid item to report")
		h.v.HandleErrors(w, r, errors.NewNotFound(err, ""))
	}
	err = repo.ReportItem(ctx, *loggedAccount, p, &reason)
	if err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}
	url := ItemPermaLink(&p)

	backUrl := r.Header.Get("Referer")
	if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL) {
		url = fmt.Sprintf("%s#li-%s", backUrl, p.Hash)
	}
	h.v.Redirect(w, r, url, http.StatusFound)
}
