package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/mariusor/go-littr/internal/log"
	"github.com/openshift/osin"
	"github.com/writeas/go-nodeinfo"
	"golang.org/x/oauth2"
)

func enhanceItem(c *Cursor, n *Item) {
	if c == nil || len(c.items) == 0 || n == nil || n.Parent == nil || !n.Parent.IsValid() {
		return
	}
	np, npok := n.Parent.(*Item)
	if !npok {
		return
	}
	parent := getItemFromList(np, c.items)
	if parent == nil || !parent.IsValid() {
		return
	}
	pi, piok := parent.(*Item)
	if !piok {
		return
	}
	n.Parent = pi
	n.OP = pi
	if pi.SubmittedBy.IsValid() {
		if len(n.Metadata.To) == 0 {
			n.Metadata.To = make(AccountCollection, 0)
		}
		n.Metadata.To = append(n.Metadata.To, *pi.SubmittedBy)
	}
	if pi.Private() {
		n.MakePrivate()
	}
	if pi.OP != nil && pi.OP.IsValid() {
		n.OP = pi.OP
	} else {
		n.OP = pi.Parent
	}
	return
}

// HandleSubmit handles POST /submit requests
// HandleSubmit handles POST /~handler/hash requests
// HandleSubmit handles POST /year/month/day/hash requests
// HandleSubmit handles POST /~handler/hash/edit requests
// HandleSubmit handles POST /year/month/day/hash/edit requests
func (h *handler) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)

	var (
		n        Item
		err      error
		saveVote = true
	)

	c := ContextCursor(r.Context())
	if path.Base(r.URL.Path) == "edit" {
		saveVote = false
		n = *ContextItem(r.Context())
	}

	if err = updateItemFromRequest(r, *acc, &n); err != nil {
		h.errFn(log.Ctx{"err": err.Error()})("Error: wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	enhanceItem(c, &n)
	repo := h.storage
	if n, err = repo.SaveItem(r.Context(), n); err != nil {
		h.errFn(log.Ctx{"err": err.Error()})("unable to save item")
		h.v.HandleErrors(w, r, err)
		return
	}

	if saveVote {
		v := Vote{
			SubmittedBy: acc,
			Item:        &n,
			Weight:      1 * ScoreMultiplier,
		}
		if _, err := repo.SaveVote(r.Context(), v); err != nil {
			h.errFn(log.Ctx{
				"err":    err.Error(),
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
			})("unable to save vote for item")
		}
	}
	acc.Metadata.InvalidateOutbox()
	h.v.Redirect(w, r, ItemPermaLink(&n), http.StatusSeeOther)
}

// HandleModerationDelete serves /moderation/{hash}/rm GET request
func (h *handler) HandleModerationDelete(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)
	repo := h.storage

	cur := ContextCursor(r.Context())
	if cur == nil || cur.total == 0 {
		url := r.URL
		url.Path = path.Dir(path.Dir(url.Path))
		h.v.Redirect(w, r, url.RequestURI(), http.StatusTemporaryRedirect)
		return
	}
	var mod *ModerationOp
	for _, m := range cur.items {
		if op, ok := m.(*ModerationOp); ok {
			mod = op
		}
	}

	// we operate on the current item as the application
	repo.WithAccount(repo.app)

	backUrl := r.Header.Get("Referer")
	if _, err := repo.ModerateDelete(r.Context(), *mod, acc); err != nil {
		h.errFn(log.Ctx{"err": err})("unable to delete item")
		h.v.addFlashMessage(Error, w, r, "unable to delete item")
	}

	acc.Metadata.InvalidateOutbox()
	h.v.Redirect(w, r, backUrl, http.StatusFound)
}

// HandleModerationDelete serves /moderation/{hash}/discuss GET request
func (h *handler) HandleModerationDiscuss(w http.ResponseWriter, r *http.Request) {
}

// HandleDelete serves /{year}/{month}/{day}/{hash}/rm POST request
// HandleDelete serves /~{handle}/rm GET request
func (h *handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)
	repo := h.storage
	iri := objects.IRI(h.storage.fedbox.Service()).AddPath(chi.URLParam(r, "hash"))

	p, err := repo.LoadItem(r.Context(), iri)
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
	if p, err = repo.SaveItem(r.Context(), p); err != nil {
		h.v.addFlashMessage(Error, w, r, "unable to delete item as current user")
	}

	acc.Metadata.InvalidateOutbox()
	h.v.Redirect(w, r, url, http.StatusFound)
}

// HandleVoting serves /{year}/{month}/{day}/{hash}/{direction} request
// HandleVoting serves /~{handle}/{direction} request
func (h *handler) HandleVoting(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)
	repo := h.storage

	p, err := ItemFromContext(r.Context(), repo, chi.URLParam(r, "hash"))
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
		if _, err := repo.SaveVote(r.Context(), v); err != nil {
			h.errFn(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
				"error":  err,
			})("Error: Unable to save vote")
			h.v.addFlashMessage(Error, w, r, "Unable to save vote")
		} else {
			h.v.saveAccountToSession(w, r, acc)
		}
	} else {
		h.v.addFlashMessage(Error, w, r, "unable to vote as current user")
	}
	acc.Metadata.InvalidateOutbox()
	h.v.Redirect(w, r, url, http.StatusFound)
}

func (h *handler) FollowAccount(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)
	repo := h.storage
	var err error
	toFollow := ContextAuthors(r.Context())
	if len(toFollow) == 0 {
		h.v.HandleErrors(w, r, errors.NotFoundf("account not found"))
		return
	}
	fol := toFollow[0]
	// todo(marius): load follow reason from POST request so we can show it to the followed user
	if err = repo.FollowAccount(r.Context(), *acc, fol, nil); err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	acc.Metadata.InvalidateOutbox()
	h.v.Redirect(w, r, AccountPermaLink(&fol), http.StatusSeeOther)
}

func (h *handler) HandleFollowResponseRequest(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)
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
			IRI: AccountsIRIFilter(follower),
		},
		Object: &Filters{
			IRI: AccountsIRIFilter(*acc),
		},
	}
	// todo(marius): load response reason from POST request so we can show it to the followed user
	followRequests, cnt, err := repo.LoadFollowRequests(r.Context(), acc, ff)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	if cnt == 0 {
		h.v.HandleErrors(w, r, errors.NotFoundf("follow request not found"))
		return
	}
	follow := followRequests[0]
	if err = repo.SendFollowResponse(r.Context(), follow, accept, nil); err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	acc.Metadata.InvalidateOutbox()
	backUrl := r.Header.Get("Referer")
	h.v.Redirect(w, r, backUrl, http.StatusSeeOther)
}

const (
	DiasporaProfile = "http://nodeinfo.diaspora.software/ns/schema"
	Mastodon        = "mastodon"
)

func (r *repository) loadInstanceActorFromIRI(ctx context.Context, iri vocab.IRI) (*vocab.Actor, error) {
	actor, err := r.fedbox.Actor(ctx, iri)
	if err == nil {
		return actor, nil
	}
	nodeInfoURL := fmt.Sprintf("%s/.well-known/nodeinfo", iri)
	loadFromURL := func(url string, what any) error {
		resp, err := r.fedbox.client.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		var body []byte
		if body, err = ioutil.ReadAll(resp.Body); err != nil {
			return err
		}
		if resp.StatusCode != http.StatusGone && resp.StatusCode >= http.StatusBadRequest {
			errs, err := errors.UnmarshalJSON(body)
			if err == nil {
				for _, retErr := range errs {
					err = fmt.Errorf("%w", retErr)
				}
			}
			return errors.WrapWithStatus(resp.StatusCode, err, "invalid status received when trying to load remote account")
		}
		if err := json.Unmarshal(body, what); err != nil {
			return err
		}
		return nil
	}
	meta := node{}
	if err := loadFromURL(nodeInfoURL, &meta); err != nil {
		return nil, err
	}

	ni := nodeinfo.NodeInfo{}
	for _, l := range meta.Links {
		if strings.Contains(l.Rel, DiasporaProfile) {
			if err := loadFromURL(l.Href, &ni); err != nil {
				return nil, err
			}
		}
	}
	if ni.Software.Name == Mastodon || ni.Software.Name == softwareName {
		u, err := iri.URL()
		if err != nil {
			return nil, err
		}
		webFingerURL := fmt.Sprintf("%s/.well-known/webfinger?resource=acct:%s@%s", iri, u.Host, u.Host)
		wf := node{}
		if err := loadFromURL(webFingerURL, &wf); err != nil {
			return nil, err
		}
		for _, l := range wf.Links {
			if l.Rel == selfName && l.Type == client.ContentTypeActivityJson {
				return r.fedbox.Actor(ctx, vocab.IRI(l.Href))
			}
		}
	}
	return nil, errors.Errorf("unable to find a suitable instance url")
}

func (h *handler) HandleFollowInstanceRequest(w http.ResponseWriter, r *http.Request) {
	instanceURL := r.FormValue("url")
	if len(instanceURL) == 0 {
		h.v.HandleErrors(w, r, errors.NotValidf("Empty instance URL"))
		return
	}
	if instanceURL == h.conf.APIURL {
		h.v.HandleErrors(w, r, errors.NotValidf("Invalid instance URL, trying to add already configured instance"))
		return
	}
	repo := ContextRepository(r.Context())
	// Load instance actor
	fol, err := repo.loadInstanceActorFromIRI(context.TODO(), vocab.IRI(instanceURL))
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	if fol.PublicKey.ID == "" {
		// NOTE(marius): if the actor that we want to follow with doesn't have a public key, it can't federate
		h.v.HandleErrors(w, r, errors.NotValidf("Current instance doesn't support federation"))
		return
	}
	// we operate on the current item as the application
	repo.WithAccount(repo.app)

	acc := repo.loadAPPerson(*repo.app)
	if err = repo.FollowActor(r.Context(), acc, fol, nil); err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	h.v.addFlashMessage(Success, w, r, fmt.Sprintf("Successfully sent follow request to instance %q", instanceURL))
	backUrl := r.Header.Get("Referer")
	h.v.Redirect(w, r, backUrl, http.StatusSeeOther)
}

// BlockAccount processes a report request received at /~{handle}/block
func (h *handler) BlockAccount(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)

	reason, err := ContentFromRequest(r, *acc)
	if err != nil {
		h.errFn(log.Ctx{"before": err})("wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	repo := h.storage
	reason.Metadata.Tags = loadTagsIfExisting(repo, r.Context(), reason.Metadata.Tags)

	toBlock := ContextAuthors(r.Context())
	if len(toBlock) == 0 {
		h.v.HandleErrors(w, r, errors.NotFoundf("account not found"))
		return
	}
	block := toBlock[0]
	if err = repo.BlockAccount(r.Context(), *acc, block, &reason); err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	acc.Metadata.InvalidateOutbox()
	h.v.Redirect(w, r, PermaLink(&block), http.StatusSeeOther)
}

// BlockItem processes a block request received at /~{handle}/{hash}/block
func (h *handler) BlockItem(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)

	reason, err := ContentFromRequest(r, *acc)
	if err != nil {
		h.errFn(log.Ctx{"before": err})("wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	repo := h.storage
	reason.Metadata.Tags = loadTagsIfExisting(repo, r.Context(), reason.Metadata.Tags)

	p, err := ItemFromContext(r.Context(), repo, chi.URLParam(r, "hash"))
	if err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}
	if err = repo.BlockItem(r.Context(), *acc, p, &reason); err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	acc.Metadata.InvalidateOutbox()
	h.v.Redirect(w, r, PermaLink(&p), http.StatusSeeOther)
}

// ReportAccount processes a report request received at /~{handle}/block
func (h *handler) ReportAccount(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)

	reason, err := ContentFromRequest(r, *acc)
	if err != nil {
		h.errFn(log.Ctx{"before": err})("Error: wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	repo := h.storage
	reason.Metadata.Tags = loadTagsIfExisting(repo, r.Context(), reason.Metadata.Tags)

	byHandleAccounts := ContextAuthors(r.Context())
	if len(byHandleAccounts) == 0 {
		h.v.HandleErrors(w, r, errors.NotFoundf("account not found"))
		return
	}
	p := byHandleAccounts[0]
	if err = repo.ReportAccount(r.Context(), *acc, p, &reason); err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}
	url := AccountPermaLink(&p)

	backUrl := r.Header.Get("Referer")
	if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL) {
		url = fmt.Sprintf("%s#li-%s", backUrl, p.Hash)
	}
	acc.Metadata.InvalidateOutbox()
	h.v.Redirect(w, r, url, http.StatusFound)
}

// ReportItem processes a report request received at /~{handle}/{hash}/bad
func (h *handler) ReportItem(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)

	reason, err := ContentFromRequest(r, *acc)
	if err != nil {
		h.errFn(log.Ctx{"before": err})("Error: wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}

	repo := h.storage
	reason.Metadata.Tags = loadTagsIfExisting(repo, r.Context(), reason.Metadata.Tags)

	p, err := ItemFromContext(r.Context(), repo, chi.URLParam(r, "hash"))
	if err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}
	if err = repo.ReportItem(r.Context(), *acc, p, &reason); err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}
	url := ItemPermaLink(&p)

	backUrl := r.Header.Get("Referer")
	if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL) {
		url = fmt.Sprintf("%s#li-%s", backUrl, p.Hash)
	}
	acc.Metadata.InvalidateOutbox()
	h.v.Redirect(w, r, url, http.StatusFound)
}

const SessionUserKey = "__current_acct"

func FilterAccountByHandle(handle string) *Filters {
	return &Filters{
		Name: CompStrs{EqualsString(handle)},
		Type: ActivityTypesFilter(ValidActorTypes...),
	}
}

// HandleLogin handles POST /login requests
func (h *handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	pw := r.PostFormValue("pw")
	handle := r.PostFormValue("handle")
	state := r.PostFormValue("state")

	repo := ContextRepository(r.Context())

	config := h.conf.GetOauth2Config("fedbox", h.conf.BaseURL)
	// Try to load actor from handle
	accts, err := repo.accounts(r.Context(), FilterAccountByHandle(handle))

	handleErr := func(msg string, f log.Ctx) {
		h.errFn(f)("Error: %s", err)
		h.v.addFlashMessage(Error, w, r, msg)
		h.v.Redirect(w, r, "/login", http.StatusSeeOther)
	}
	lCtx := log.Ctx{
		"handle": handle,
		"client": config.ClientID,
		"state":  state,
	}
	if err != nil || len(accts) == 0 {
		if err == nil {
			err = errors.NotFoundf(handle)
		}
		lCtx["err"] = err.Error()
		handleErr("Login failed: invalid username or password", lCtx)
		return
	}

	var (
		tok  *oauth2.Token
		acct = AnonymousAccount
	)
	for _, cur := range accts {
		if tok, err = config.PasswordCredentialsToken(r.Context(), cur.Metadata.ID, pw); tok != nil {
			acct = cur
			acct.Metadata.OAuth.Provider = "fedbox"
			acct.Metadata.OAuth.Token = tok
			break
		}
	}
	if !acct.IsLogged() {
		if err == nil {
			err = errors.Errorf("unable to authenticate account")
		}
		lCtx["err"] = err.Error()
		handleErr("Login failed: invalid username or password", lCtx)
		return
	}
	s, err := h.v.s.get(w, r)
	if err != nil {
		lCtx["err"] = err.Error()
		handleErr("Login failed: unable to save session", lCtx)
		return
	}
	s.Values[SessionUserKey] = acct
	h.v.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleLogout serves /logout requests
func (h *handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	h.v.s.clear(w, r)
	backUrl := "/"
	if refUrl := r.Header.Get("Referer"); HostIsLocal(refUrl) && !strings.Contains(refUrl, "followed") {
		backUrl = refUrl
	}
	// TODO(marius): this doesn't need as drastic cache clear as this, we need to implement a prefix based clear
	h.storage.cache.remove()
	h.v.Redirect(w, r, backUrl, http.StatusSeeOther)
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

func (h *handler) ValidateModerator() Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			acc := loggedAccount(r)
			url := r.URL
			if !acc.IsModerator() {
				url.Path = path.Dir(url.Path)
				h.v.addFlashMessage(Error, w, r, "Current user is not allowed to moderate")
				h.v.Redirect(w, r, url.RequestURI(), http.StatusTemporaryRedirect)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

func (h *handler) ValidateItemAuthor(op string) Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			acc := loggedAccount(r)
			hash := chi.URLParam(r, "hash")
			url := r.URL
			action := path.Base(url.Path)
			if len(hash) > 0 && action != hash {
				repo := h.storage
				p, err := ItemFromContext(r.Context(), repo, chi.URLParam(r, "hash"))
				if err != nil {
					ctxtErr(next, w, r, errors.NewNotFound(err, "item"))
					return
				}
				if p.SubmittedBy.Hash != acc.Hash {
					url.Path = path.Dir(url.Path)
					h.v.addFlashMessage(Error, w, r, fmt.Sprintf("Unable to %s item as current user", op))
					h.v.Redirect(w, r, url.RequestURI(), http.StatusTemporaryRedirect)
					return
				}
				next.ServeHTTP(w, r)
			}
		}
		return http.HandlerFunc(fn)
	}
}

func ItemFromContext(ctx context.Context, repo *repository, hash string) (Item, error) {
	if p := ContextItem(ctx); p != nil {
		return *p, nil
	}
	return Item{}, errors.NotFoundf(hash)
}

// HandleItemRedirect serves /i/{hash} request
func (h *handler) HandleItemRedirect(w http.ResponseWriter, r *http.Request) {
	repo := h.storage

	p, err := ItemFromContext(r.Context(), repo, chi.URLParam(r, "hash"))
	if err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
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
	config := h.conf.GetOauth2Config("fedbox", h.conf.BaseURL)
	config.Scopes = []string{scopeAnonymousUserCreate}
	param := oauth2.SetAuthURLParam("actor", invitee.AP().GetLink().String())
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

// HandleCreateInvitation handles POST ~handle/invite requests
func (h *handler) HandleCreateInvitation(w http.ResponseWriter, r *http.Request) {
	if !h.conf.UserInvitesEnabled {
		h.v.HandleErrors(w, r, errors.BadRequestf("unable to invite user"))
		return
	}

	acc := loggedAccount(r)
	invitee, err := h.storage.SaveAccount(r.Context(), Account{CreatedBy: acc})
	if err != nil {
		h.v.HandleErrors(w, r, errors.NewBadRequest(err, "unable to save account"))
		return
	}
	if !invitee.IsValid() || !invitee.HasMetadata() || invitee.Metadata.ID == "" {
		h.v.HandleErrors(w, r, errors.NewBadRequest(err, "invalid account saved"))
		return
	}

	acc.Metadata.InvalidateOutbox()
	h.v.addFlashMessage(Info, w, r, "Invitation generated successfully.\nYou can now send an email to the person you want to invite by clicking the envelope icon.")
	h.v.Redirect(w, r, PermaLink(acc), http.StatusMovedPermanently)
}

// HandleRegister handles POST /register requests
func (h *handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	a, err := h.accountFromPost(r)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}

	repo := ContextRepository(r.Context())
	maybeExists, err := repo.account(r.Context(), FilterAccountByHandle(a.Handle))
	if err != nil && !errors.IsNotFound(err) {
		h.logger.WithContext(log.Ctx{"handle": a.Handle, "err": err}).Warnf("error when trying to load account")
		h.v.HandleErrors(w, r, errors.NewBadRequest(err, "error when trying to load account %s", a.Handle))
		return
	}
	if maybeExists.IsValid() {
		h.v.HandleErrors(w, r, errors.BadRequestf("account %s already exists", a.Handle))
		return
	}

	app := h.storage.app
	a.CreatedBy = app
	a, err = h.storage.WithAccount(app).SaveAccount(r.Context(), a)
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
	config := h.conf.GetOauth2Config("fedbox", h.conf.BaseURL)
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
	pwChURL := fmt.Sprintf("%s/oauth/pw", h.storage.BaseURL())
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
			Status:     http.StatusInternalServerError,
			StatusText: "Oops!!",
			Title:      "Oops!!",
		}
	}
	if cursor := ContextCursor(r.Context()); cursor != nil {
		if mod, ok := m.(Paginator); ok {
			mod.SetCursor(cursor)
		}
	}
	if err := h.v.RenderTemplate(r, w, m.Template(), m); err != nil {
		h.v.HandleErrors(w, r, err)
	}
}

// Repository middleware
func (h handler) Repository(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), RepositoryCtxtKey, h.storage)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}
