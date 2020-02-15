package app

import (
	"encoding/json"
	"fmt"
	"github.com/go-ap/errors"
	"github.com/gorilla/csrf"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/mariusor/qstring"
	"github.com/openshift/osin"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"io/ioutil"
	"math"
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
	n, err := ContentFromRequest(r, *acc)
	if err != nil {
		h.logger.WithContext(log.Ctx{
			"before": err,
		}).Error("wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	saveVote := true

	repo := h.storage
	if n.Parent.IsValid() {
		if n.Parent.SubmittedAt.IsZero() {
			if p, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{n.Parent.Hash}}}); err == nil {
				n.Parent = &p
				if p.OP != nil {
					n.OP = p.OP
				}
			}
		}
		if len(n.Metadata.To) == 0 {
			n.Metadata.To = make([]*Account, 0)
		}
		n.Metadata.To = append(n.Metadata.To, n.Parent.SubmittedBy)
		if n.Parent.Private() {
			n.MakePrivate()
			saveVote = false
		}
	}

	if len(n.Hash) > 0 {
		if p, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{n.Hash}}}); err == nil {
			n.Title = p.Title
		}
		saveVote = false
	}
	n, err = repo.SaveItem(n)
	if err != nil {
		h.logger.WithContext(log.Ctx{
			"before": err,
		}).Error("unable to save item")
		h.v.HandleErrors(w, r, err)
		return
	}

	if saveVote {
		v := Vote{
			SubmittedBy: acc,
			Item:        &n,
			Weight:      1 * ScoreMultiplier,
		}
		if _, err := repo.SaveVote(v); err != nil {
			h.logger.WithContext(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
			}).Error(err.Error())
		}
	}
	h.v.Redirect(w, r, ItemPermaLink(n), http.StatusSeeOther)
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
	hash := chi.URLParam(r, "hash")

	repo := h.storage
	p, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{Hash(hash)}}})
	if err != nil {
		h.logger.Error(err.Error())
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}

	url := ItemPermaLink(p)
	backUrl := r.Header.Get("Referer")
	if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL) {
		url = fmt.Sprintf("%s#item-%s", backUrl, p.Hash)
	}
	p.Delete()
	if p, err = repo.SaveItem(p); err != nil {
		h.v.addFlashMessage(Error, r, "unable to delete item as current user")
	}

	h.v.Redirect(w, r, url, http.StatusFound)
}

// HandleReport serves /{year}/{month}/{day}/{hash}/bad POST request
// HandleReport serves /~{handle}/{hash}/bad request
func (h *handler) HandleReport(w http.ResponseWriter, r *http.Request) {
	m := &contentModel{}
	h.v.RenderTemplate(r, w, "new", m)
}

// ShowReport serves /{year}/{month}/{day}/{hash}/bad GET request
// ShowReport serves /~{handle}/{hash}/bad request
func (h *handler) ShowReport(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")

	repo := h.storage
	p, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{Hash(hash)}}})
	if err != nil {
		h.logger.Error(err.Error())
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}
	m := &contentModel{
		Title:   fmt.Sprintf("Report %s", p.Title),
		Content: p,
	}
	h.v.RenderTemplate(r, w, "new", m)
}

// HandleVoting serves /{year}/{month}/{day}/{hash}/{direction} request
// HandleVoting serves /~{handle}/{direction} request
func (h *handler) HandleVoting(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")

	repo := h.storage
	p, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{Hash(hash)}}})
	if err != nil {
		h.logger.Error(err.Error())
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
	url := ItemPermaLink(p)

	acc := loggedAccount(r)
	if acc.IsLogged() {
		backUrl := r.Header.Get("Referer")
		if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL) {
			url = fmt.Sprintf("%s#item-%s", backUrl, p.Hash)
		}
		v := Vote{
			SubmittedBy: acc,
			Item:        &p,
			Weight:      multiplier * ScoreMultiplier,
		}
		if _, err := repo.SaveVote(v); err != nil {
			h.logger.WithContext(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
				"error":  err,
			}).Error("Unable to save vote")
			h.v.addFlashMessage(Error, r, err.Error())
		}
	} else {
		h.v.addFlashMessage(Error, r, "unable to vote as current user")
	}
	h.v.Redirect(w, r, url, http.StatusFound)
}

// ShowItem serves /~{handle}/{hash} request
// ShowItem serves /~{handle}/{hash}/edit request
// ShowItem serves /{year}/{month}/{day}/{hash} request
// ShowItem serves /{year}/{month}/{day}/{hash}/edit request
func (h *handler) ShowItem(w http.ResponseWriter, r *http.Request) {
	items := make([]Item, 0)

	m := &contentModel{}
	repo := h.storage
	auth := ContextAuthor(r.Context())
	handle := auth.Handle
	hash := chi.URLParam(r, "hash")
	f := Filters{
		LoadItemsFilter: LoadItemsFilter{
			Key: Hashes{Hash(hash)},
		},
	}
	if !HashesEqual(auth.Hash, AnonymousHash) {
		f.LoadItemsFilter.AttributedTo = Hashes{auth.Hash}
	}

	i, err := repo.LoadItem(f)
	if err != nil {
		h.logger.WithContext(log.Ctx{
			"handle": handle,
			"hash":   hash,
		}).Error(err.Error())
		h.v.HandleErrors(w, r, errors.NotFoundf("Item %q", hash))
		return
	}
	if !i.Deleted() && len(i.Data)+len(i.Title) == 0 {
		datLen := int(math.Min(12.0, float64(len(i.Data))))
		h.logger.WithContext(log.Ctx{
			"handle":      handle,
			"hash":        hash,
			"title":       i.Title,
			"content":     i.Data[0:datLen],
			"content_len": len(i.Data),
		}).Warn("Item deleted or empty")
		h.v.HandleErrors(w, r, errors.NotFoundf("Item %q", hash))
		return
	}
	m.Content = i
	url := r.URL
	maybeEdit := path.Base(url.Path)

	account := loggedAccount(r)
	if maybeEdit != hash && maybeEdit == Edit {
		if !HashesEqual(m.Content.SubmittedBy.Hash, account.Hash) {
			url.Path = path.Dir(url.Path)
			h.v.Redirect(w, r, url.RequestURI(), http.StatusFound)
			return
		}
		m.Content.Edit = true
	}

	items = append(items, i)
	allComments := make(ItemCollection, 1)
	allComments[0] = m.Content

	filter := Filters{
		LoadItemsFilter: LoadItemsFilter{
			Depth: 10,
		},
		MaxItems: MaxContentItems,
		Page:     1,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}

	if i.OP.IsValid() {
		if id, ok := BuildIDFromItem(*i.OP); ok {
			filter.Context = []string{string(id)}
		}
	}
	if filter.Context == nil {
		filter.Context = []string{m.Content.Hash.String()}
	}
	contentItems, _, err := repo.LoadItems(filter)
	if len(contentItems) >= filter.MaxItems {
		m.after = contentItems[len(contentItems)-1].Hash
	}
	if filter.Page > 1 {
		m.before = contentItems[0].Hash
	}
	if err != nil {
		h.logger.Error(err.Error())
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "" /*, errors.ErrorStack(err)*/))
		return
	}
	allComments = append(allComments, contentItems...)

	if i.Parent.IsValid() && i.Parent.SubmittedAt.IsZero() {
		if p, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{i.Parent.Hash}}}); err == nil {
			i.Parent = &p
			if p.OP != nil {
				i.OP = p.OP
			} else {
				i.OP = &p
			}
		}
	}

	reparentComments(allComments)
	addLevelComments(allComments)
	removeCurElementParentComments(&allComments)

	if account.IsLogged() {
		account.Votes, _, err = repo.LoadVotes(Filters{
			LoadVotesFilter: LoadVotesFilter{
				AttributedTo: []Hash{account.Hash},
				ItemKey:      allComments.getItemsHashes(),
			},
			MaxItems: MaxContentItems,
		})
		if err != nil {
			h.logger.Error(err.Error())
		}
	}

	if len(m.Title) > 0 {
		m.Title = fmt.Sprintf("%s", i.Title)
	} else {
		// FIXME(marius): we lost the handler of the account
		m.Title = fmt.Sprintf("%s comment", genitive(m.Content.SubmittedBy.Handle))
	}
	h.v.RenderTemplate(r, w, "content", m)
}

func (h *handler) FollowAccount(w http.ResponseWriter, r *http.Request) {
	loggedAccount := loggedAccount(r)
	if !loggedAccount.IsValid() {
		err := errors.Unauthorizedf("invalid logged account")
		h.logger.Error(err.Error())
		h.v.HandleErrors(w, r, err)
		return
	}

	repo := h.storage
	var err error
	toFollow := ContextAuthor(r.Context())
	if toFollow == nil {
		h.v.HandleErrors(w, r, errors.NotFoundf("account not found"))
		return
	}
	err = repo.FollowAccount(*loggedAccount, *toFollow)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	h.v.Redirect(w, r, AccountPermaLink(*toFollow), http.StatusSeeOther)
}

func (h *handler) HandleFollowRequest(w http.ResponseWriter, r *http.Request) {
	loggedAccount := loggedAccount(r)
	if !loggedAccount.IsValid() {
		err := errors.Unauthorizedf("invalid logged account")
		h.logger.Error(err.Error())
		h.v.HandleErrors(w, r, err)
		return
	}

	repo := h.storage
	follower := ContextAuthor(r.Context())
	if follower == nil {
		h.v.HandleErrors(w, r, errors.NotFoundf("account not found"))
		return
	}
	accept := false
	action := chi.URLParam(r, "action")
	if action == "accept" {
		accept = true
	}

	followRequests, cnt, err := repo.LoadFollowRequests(loggedAccount, Filters{
		LoadFollowRequestsFilter: LoadFollowRequestsFilter{
			Actor: Hashes{Hash(follower.Metadata.ID)},
			On:    Hashes{Hash(loggedAccount.Metadata.ID)},
		},
	})
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	if cnt == 0 {
		h.v.HandleErrors(w, r, errors.NotFoundf("follow request not found"))
		return
	}
	follow := followRequests[0]
	err = repo.SendFollowResponse(follow, accept)
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

	config := GetOauth2Config("fedbox", h.conf.BaseURL)
	// Try to load actor from handle
	acct, err := h.storage.LoadAccount(Filters{
		LoadAccountsFilter: LoadAccountsFilter{
			Handle:  []string{handle},
			Deleted: []bool{false},
		},
	})
	if err != nil {
		h.logger.WithContext(logrus.Fields{
			"handle": handle,
			"client": config.ClientID,
			"state":  state,
		}).Error(err.Error())
		h.v.addFlashMessage(Error, r, fmt.Sprintf("Login failed: %s", err))
		h.v.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tok, err := config.PasswordCredentialsToken(r.Context(), handle, pw)
	if err != nil {
		h.logger.WithContext(logrus.Fields{
			"handle": handle,
			"client": config.ClientID,
			"state":  state,
			"error":  err,
		}).Error("login failed")
		h.v.addFlashMessage(Error, r, "Login failed: invalid username or password")
		h.v.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if tok == nil {
		h.logger.WithContext(logrus.Fields{
			"handle": handle,
			"client": config.ClientID,
			"state":  state,
		}).Errorf("nil token received")
		h.v.addFlashMessage(Error, r, "Login failed: wrong handle or password")
		h.v.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	acct.Metadata.OAuth.Provider = "fedbox"
	acct.Metadata.OAuth.Token = tok.AccessToken
	acct.Metadata.OAuth.TokenType = tok.TokenType
	acct.Metadata.OAuth.RefreshToken = tok.RefreshToken
	s, _ := h.v.s.get(r)
	s.Values[SessionUserKey] = acct
	h.v.Redirect(w, r, "/", http.StatusSeeOther)
}

// ShowLogin serves GET /login requests
func (h *handler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	a := loggedAccount(r)

	m := loginModel{Title: "Login"}
	m.Account = *a

	h.v.RenderTemplate(r, w, m.Template(), m)
}

// HandleLogout serves /logout requests
func (h *handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	s, err := h.v.s.get(r)
	if err != nil {
		h.logger.Error(err.Error())
	}
	s.Values[SessionUserKey] = nil
	backUrl := "/"
	if r.Header.Get("Referer") != "" {
		backUrl = r.Header.Get("Referer")
	}
	h.v.Redirect(w, r, backUrl, http.StatusSeeOther)
}

// ShowSubmit serves GET /submit request
func (h *handler) ShowSubmit(w http.ResponseWriter, r *http.Request) {
	h.v.RenderTemplate(r, w, "new", contentModel{Title: "New submission"})
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
				h.logger.Errorf("%s", e)
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
		acc := loggedAccount(r)
		hash := chi.URLParam(r, "hash")
		url := r.URL
		action := path.Base(url.Path)
		if len(hash) > 0 && action != hash {
			repo := h.storage
			m, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{Hash(hash)}}})
			if err != nil {
				h.logger.Error(err.Error())
				h.v.HandleErrors(w, r, errors.NewNotFound(err, "item"))
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
	p, err := repo.LoadItem(Filters{
		LoadItemsFilter: LoadItemsFilter{
			Key: Hashes{Hash(chi.URLParam(r, "hash"))},
		},
		MaxItems: 1,
	})
	if err != nil {
		h.v.HandleErrors(w, r, errors.NewNotValid(err, "oops!"))
		return
	}
	url := ItemPermaLink(p)
	h.v.Redirect(w, r, url, http.StatusMovedPermanently)
}

// ShowRegister serves GET /register requests
func (h *handler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	m := registerModel{}
	h.v.RenderTemplate(r, w, m.Template(), m)
}

var scopeAnonymousUserCreate = "anonUserCreate"

// HandleRegister handles POST /register requests
func (h *handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	a, err := accountFromPost(r, h.logger)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}

	maybeExists, err := h.storage.LoadAccount(Filters{
		LoadAccountsFilter: LoadAccountsFilter{
			Handle: []string{a.Handle},
		},
	})
	notFound := errors.NotFoundf("")
	if err != nil && !notFound.As(err) {
		h.v.HandleErrors(w, r, errors.NewBadRequest(err, "unable to create"))
		return
	}
	if maybeExists.IsValid() {
		h.v.HandleErrors(w, r, errors.BadRequestf("account %s already exists", a.Handle))
		return
	}

	acc := loggedAccount(r)
	if !acc.IsLogged() {
		acc = h.storage.app
	}
	a.CreatedBy = acc
	h.storage.WithAccount(acc)
	*a, err = h.storage.SaveAccount(*a)
	if err != nil {
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
		h.logger.Error(err.Error())
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

// HandleShow serves / request
func (h *handler) HandleShow(w http.ResponseWriter, r *http.Request) {
	m := ContextModel(r.Context())
	if m == nil {
		h.v.HandleErrors(w, r, errors.Errorf("Oops!!"))
		return
	}
	cursor := ContextCursor(r.Context())
	if cursor == nil {
		h.v.HandleErrors(w, r, errors.Errorf("Oops cursor!!"))
		return
	}
	if mod, ok := m.(Paginator); ok {
		mod.SetCursor(cursor)
	}
	if acc := loggedAccount(r); acc != nil {
		repo := ContextRepository(r.Context())
		err := repo.loadAccountVotes(acc, cursor.items.Items())
		if err != nil {
			h.v.HandleErrors(w, r, errors.Errorf("unable to load account votes"))
			return
		}
	}
	if err := h.v.RenderTemplate(r, w, m.Template(), m); err != nil {
		h.v.HandleErrors(w, r, err)
	}
}
