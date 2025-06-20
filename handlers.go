package brutalinks

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	xerrors "errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"git.sr.ht/~mariusor/box"
	log "git.sr.ht/~mariusor/lw"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/client/credentials"
	"github.com/go-ap/errors"
	"github.com/go-ap/filters"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
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
		u := r.URL
		u.Path = path.Dir(path.Dir(u.Path))
		h.v.Redirect(w, r, u.RequestURI(), http.StatusTemporaryRedirect)
		return
	}
	var mod *ModerationOp
	for _, m := range cur.items {
		if op, ok := m.(*ModerationOp); ok {
			mod = op
		}
	}
	if mod != nil {
		if _, err := repo.ModerateDelete(r.Context(), *mod, acc); err != nil {
			h.errFn(log.Ctx{"err": err})("unable to delete item")
			h.v.addFlashMessage(Error, w, r, "unable to delete item")
		}

		acc.Metadata.InvalidateOutbox()
	}

	backUrl := r.Header.Get("Referer")
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

	checks := filters.All(
		filters.IRILike(chi.URLParam(r, "hash")),
		filters.SameAttributedTo(acc.AP().GetLink()),
	)

	res, err := repo.b.Search(checks)
	if err != nil || len(res) != 1 {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}
	li := res[0]
	it, ok := li.(vocab.Item)
	if !ok {
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}

	p := Item{}
	_ = p.FromActivityPub(it)
	p.SubmittedBy = acc

	u := ItemPermaLink(&p)
	backUrl := r.Header.Get("Referer")
	if !strings.Contains(backUrl, u) && strings.Contains(backUrl, Instance.BaseURL.String()) {
		u = fmt.Sprintf("%s#li-%s", backUrl, p.Hash)
	}
	p.Delete()
	if p, err = repo.SaveItem(r.Context(), p); err != nil {
		h.v.addFlashMessage(Error, w, r, "unable to delete item as current user")
	}

	acc.Metadata.InvalidateOutbox()
	h.v.Redirect(w, r, u, http.StatusFound)
}

// HandleVoting serves /{year}/{month}/{day}/{hash}/{direction} request
// HandleVoting serves /~{handle}/{direction} request
func (h *handler) HandleVoting(w http.ResponseWriter, r *http.Request) {
	acc := loggedAccount(r)
	repo := h.storage

	p, err := ItemFromContext(r.Context(), repo, chi.URLParam(r, "hash"))
	if err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "Item not found"))
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
		if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL.String()) {
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
	// TODO(marius): load follow reason from POST request so we can show it to the followed user
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

	accept := false
	action := chi.URLParam(r, "action")
	if action == "accept" {
		accept = true
	}

	follow, err := FollowRequestFromContext(r.Context(), chi.URLParam(r, "hash"))
	if err != nil {
		h.errFn()("Error: %s", err)
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "Follow request not found"))
		return
	}
	if follow.Object == nil {
		h.v.HandleErrors(w, r, errors.NotFoundf("account not found"))
		return
	}
	if follow.Object.Hash.String() == repo.app.Hash.String() {
		// we operate on the current item as the application
		follow.SubmittedBy = repo.app
	} else if follow.Object.Hash.String() == acc.Hash.String() {
		follow.SubmittedBy = acc
	} else {
		h.v.HandleErrors(w, r, errors.NotFoundf("unable to reply to follow as current account"))
		return
	}

	if err = repo.SendFollowResponse(r.Context(), follow, accept, nil); err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	acc.Metadata.InvalidateOutbox()
	backUrl := r.Header.Get("Referer")
	h.v.Redirect(w, r, backUrl, http.StatusSeeOther)
}

const (
	DiasporaProfile = vocab.IRI("https://nodeinfo.diaspora.software/ns/schema")
	Mastodon        = "mastodon"
)

func (r *repository) loadWebfingerActorFromIRI(ctx context.Context, host, acct string) (*vocab.Actor, error) {
	loadFromURL := func(url string, what any) error {
		resp, err := r.fedbox.Client(nil).Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		var body []byte
		if body, err = io.ReadAll(resp.Body); err != nil {
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

	nodeInfoURL := fmt.Sprintf("%s/.well-known/webfinger?resource=acct:%s", host, acct)
	if err := loadFromURL(nodeInfoURL, &meta); err != nil {
		r.errFn(log.Ctx{"url": nodeInfoURL, "err": err.Error()})("unable to load WebFinger resource")
	}

	for _, l := range meta.Links {
		if l.Type == client.ContentTypeActivityJson {
			actor := vocab.Actor{}
			if err := loadFromURL(l.Href, &actor); err == nil {
				return &actor, nil
			}
		}
	}
	return nil, errors.Errorf("unable to find webfinger actor")

}
func (r *repository) loadInstanceActorFromIRI(ctx context.Context, iri vocab.IRI) (*vocab.Actor, error) {
	actor, err := r.fedbox.Actor(ctx, iri)
	if err == nil {
		return actor, nil
	}
	nodeInfoURL := fmt.Sprintf("%s/.well-known/nodeinfo", iri)
	loadFromURL := func(url string, what any) error {
		resp, err := r.fedbox.Client(nil).Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		var body []byte
		if body, err = io.ReadAll(resp.Body); err != nil {
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
		if vocab.IRI(l.Rel).Contains(DiasporaProfile, false) {
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
	backURL := r.Header.Get("Referer")
	if len(instanceURL) == 0 {
		h.v.HandleErrors(w, r, errors.NotValidf("Empty instance URL"))
		return
	}
	if instanceURL == h.conf.APIURL {
		h.v.Redirect(w, r, backURL, http.StatusSeeOther)
		return
	}
	repo := ContextRepository(r.Context())
	// Load instance actor
	rem, err := repo.loadInstanceActorFromIRI(context.TODO(), vocab.IRI(instanceURL))
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	if rem.PublicKey.ID == "" {
		// NOTE(marius): if the actor that we want to follow with doesn't have a public key, it can't federate
		h.v.HandleErrors(w, r, errors.NotValidf("Instance doesn't support federation: %s", instanceURL))
		return
	}

	fol := Account{}
	_ = fol.FromActivityPub(rem)

	if err = repo.FollowActor(r.Context(), *repo.app, fol, nil); err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}
	h.v.addFlashMessage(Success, w, r, fmt.Sprintf("Successfully sent a follow request to instance: %s", instanceURL))
	h.v.Redirect(w, r, backURL, http.StatusSeeOther)
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
	if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL.String()) {
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
	if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL.String()) {
		url = fmt.Sprintf("%s#li-%s", backUrl, p.Hash)
	}
	acc.Metadata.InvalidateOutbox()
	h.v.Redirect(w, r, url, http.StatusFound)
}

const SessionUserKey = "__current_acct"

func AccountByHandleCheck(handle string) filters.Check {
	return filters.All(
		filters.NameIs(handle),
		filters.HasType(ValidActorTypes...),
	)
}

func (h handler) loadAccountsByPw(ctx context.Context, accts AccountCollection, pw string) Account {
	config := h.conf.GetOauth2Config(fedboxProvider, h.conf.BaseURL)

	var (
		tok  *oauth2.Token
		acct = AnonymousAccount
	)
	for _, cur := range accts {
		if tok, _ = config.PasswordCredentialsToken(ctx, cur.Metadata.ID, pw); tok != nil {
			acct = cur
			acct.Metadata.OAuth.Provider = fedboxProvider
			acct.Metadata.OAuth.Token = tok
			break
		}
	}
	return acct
}

func publicKey(data []byte) crypto.PublicKey {
	b, _ := pem.Decode(data)
	if b == nil {
		panic("failed decoding pem")
	}
	pubKey, err := x509.ParsePKIXPublicKey(b.Bytes)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	return pubKey
}

// HandleLogin handles POST /login requests
func (h *handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	pw := r.PostFormValue("pw")
	handle := r.PostFormValue("handle")
	wf := r.PostFormValue("webfinger")

	ctx := context.Background()
	repo := ContextRepository(r.Context())

	var err error

	handleErr := func(msg string, f log.Ctx) {
		h.errFn(f)("Error: %s", msg)
		h.v.addFlashMessage(Error, w, r, msg)
		h.v.Redirect(w, r, "/login", http.StatusSeeOther)
	}

	host := h.conf.APIURL
	if len(wf) > 0 {
		if pieces := strings.Split(strings.TrimPrefix(wf, "@"), "@"); len(pieces) > 0 {
			handle = pieces[0]
			if len(pieces) > 1 {
				host = fmt.Sprintf("https://%s", pieces[1])
			}
		}
	}

	accts := make([]Account, 0)
	// Try to load actor from handle
	f := AccountByHandleCheck(handle)
	f = filters.All(f, filters.IRILike(host))
	if acc, err := repo.accounts(ctx, f); err == nil {
		accts = append(accts, acc...)
	}

	if len(wf) > 0 {
		var a *vocab.Actor
		if a, err = repo.loadWebfingerActorFromIRI(ctx, host, wf); err == nil {
			acct := Account{}
			_ = acct.FromActivityPub(a)
			accts = append(accts, acct)
		}
	}

	lCtx := log.Ctx{"handle": handle, "count": len(accts)}
	if len(accts) == 0 {
		lCtx["err"] = "unable to find account"
		handleErr("Login failed: unable to find account for authorization", lCtx)
		return
	}
	var acct Account
	if len(pw) > 0 {
		acct = h.loadAccountsByPw(ctx, accts, pw)
		if !acct.IsLogged() {
			if err == nil {
				err = errors.Errorf("unable to authenticate account")
			}
			lCtx["err"] = err.Error()
			handleErr("Login failed: invalid authentication data", lCtx)
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
	config := h.conf.GetOauth2Config(fedboxProvider, h.conf.BaseURL)
	var state string
	for _, acct = range accts {
		_ = vocab.OnActor(acct.AP(), func(a *vocab.Actor) error {
			if a.Endpoints == nil {
				return nil
			}
			if !vocab.IsNil(a.Endpoints.OauthAuthorizationEndpoint) {
				config.Endpoint.AuthURL = a.Endpoints.OauthAuthorizationEndpoint.GetLink().String()
			}
			if !vocab.IsNil(a.Endpoints.OauthTokenEndpoint) {
				config.Endpoint.TokenURL = a.Endpoints.OauthTokenEndpoint.GetLink().String()
			}
			state = genStateForAccount(acct)
			return h.v.saveAccountToSession(w, r, &acct)
		})
		h.v.Redirect(w, r, config.AuthCodeURL(state), http.StatusSeeOther)
		return
	}

	lCtx["err"] = "unable to find account"
	handleErr("Login failed: unable to authorize using account", lCtx)
}

// HandleLogout serves /logout requests
func (h *handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	_ = h.v.saveAccountToSession(w, r, &AnonymousAccount)
	backUrl := "/"
	if refUrl := r.Header.Get("Referer"); HostIsLocal(refUrl) && !strings.Contains(refUrl, "followed") {
		backUrl = refUrl
	}
	// TODO(marius): this doesn't need as drastic cache clear as this, we need to implement a prefix based clear
	h.storage.cache.remove()
	h.v.s.clear(w, r)
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
	return Item{}, errors.NotFoundf("Item not found %s", hash)
}

func ContextFollowRequest(ctx context.Context) *FollowRequest {
	if f, ok := ctx.Value(ContentCtxtKey).(*FollowRequest); ok {
		return f
	}
	return nil
}

func FollowRequestFromContext(ctx context.Context, hash string) (FollowRequest, error) {
	p := ContextFollowRequest(ctx)
	if p != nil && p.Hash.String() == hash {
		return *p, nil
	}

	if c := ContextCursor(ctx); c != nil {
		for _, it := range c.items {
			if it.Type() != FollowType {
				continue
			}
			f, ok := it.(*FollowRequest)
			if ok && strings.Contains(f.AP().GetLink().String(), hash) {
				return *f, nil
			}
		}
	}
	return FollowRequest{}, errors.NotFoundf("Item not found %s", hash)
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

// HandleChangePassword handles POST /pw requests
func (h *handler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	a, err := h.accountFromPost(r)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}

	repo := ContextRepository(r.Context())
	maybeExists, err := repo.account(r.Context(), AccountByHandleCheck(a.Handle))
	if err != nil && !errors.IsNotFound(err) {
		h.logger.WithContext(log.Ctx{"handle": a.Handle, "err": err}).Warnf("error when trying to load account")
		h.v.HandleErrors(w, r, errors.NewBadRequest(err, "error when trying to load account %s", a.Handle))
		return
	}
	if !maybeExists.IsValid() {
		h.v.HandleErrors(w, r, errors.BadRequestf("could not find account %s", a.Handle))
		return
	}

	app := h.storage.app
	a.CreatedBy = app
	a, err = h.storage.SaveAccount(r.Context(), a)
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
	config := h.conf.GetOauth2Config(fedboxProvider, h.conf.BaseURL)
	config.Scopes = []string{scopeAnonymousUserCreate}
	param := oauth2.SetAuthURLParam("actor", a.Metadata.ID)
	sessUrl := config.AuthCodeURL(csrf.Token(r), param)

	res, err := http.Get(sessUrl)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}

	var body []byte
	if body, err = io.ReadAll(res.Body); err != nil {
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
	if body, err = io.ReadAll(pwChRes.Body); err != nil {
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

// HandleRegister handles POST /register requests
func (h *handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	a, err := h.accountFromPost(r)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}

	repo := ContextRepository(r.Context())
	maybeExists, err := repo.account(r.Context(), AccountByHandleCheck(a.Handle))
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
	a, err = h.storage.SaveAccount(r.Context(), a)
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
	config := h.conf.GetOauth2Config(fedboxProvider, h.conf.BaseURL)
	config.Scopes = []string{scopeAnonymousUserCreate}
	param := oauth2.SetAuthURLParam("actor", a.Metadata.ID)
	sessUrl := config.AuthCodeURL(csrf.Token(r), param)

	res, err := http.Get(sessUrl)
	if err != nil {
		h.v.HandleErrors(w, r, err)
		return
	}

	var body []byte
	if body, err = io.ReadAll(res.Body); err != nil {
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
	if body, err = io.ReadAll(pwChRes.Body); err != nil {
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

func genStateForAccount(acct Account) string {
	buf := bytes.Buffer{}
	buf.WriteString(acct.Handle)
	buf.WriteString("\n")
	if acct.HasPublicKey() {
		buf.Write(acct.Metadata.Key.Public)
	}
	raw := strconv.AppendInt(buf.Bytes(), time.Now().UTC().Truncate(10*time.Minute).Unix(), 10)
	return fmt.Sprintf("%2x", crc32.ChecksumIEEE(raw))
}

// HandleCallback serves /auth/{provider}/callback request
func (h *handler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	redirWithError := func(errs ...error) {
		h.v.addFlashMessage(Error, w, r, xerrors.Join(errs...).Error())
		_ = h.v.saveAccountToSession(w, r, &AnonymousAccount)
		h.v.Redirect(w, r, "/login", http.StatusFound)
	}

	q := r.URL.Query()

	provider := chi.URLParam(r, "provider")

	if q.Has("error") {
		errDescriptions := q["error_description"]
		errs := make([]error, len(errDescriptions)+1)
		errs[0] = errors.Newf("%s OAuth2 error:", provider)
		for i, errDesc := range errDescriptions {
			errs[i+1] = errors.Errorf("%s", errDesc)
		}
		redirWithError(errs...)
		return
	}

	state := q.Get("state")
	code := q.Get("code")
	if len(code) == 0 {
		redirWithError(errors.Newf("%s error: Empty authentication token", provider))
		return
	}

	conf := h.conf.GetOauth2Config(provider, h.conf.BaseURL)
	tok, err := conf.Exchange(r.Context(), code)
	if err != nil {
		h.errFn(log.Ctx{"err": err.Error()})("Unable to load token")
		redirWithError(err)
		return
	}

	acct, err := h.v.loadCurrentAccountFromSession(w, r)
	if err != nil {
		h.errFn(log.Ctx{"err": err.Error()})("Failed to load account from session")
		redirWithError(errors.Newf("Failed to login with %s", provider))
		return
	} else {
		if expected := genStateForAccount(*acct); expected != state {
			h.errFn(log.Ctx{"received": state, "expected": expected})("Failed to validate state received from OAuth2 provider")
			redirWithError(errors.Newf("Failed to login with %s", provider))
			return
		}
		acct.Metadata.OAuth = OAuth{
			State:    state,
			Code:     code,
			Provider: provider,
			Token:    tok,
		}

		cred := credentials.C2S{
			IRI:  acct.AP().GetLink(),
			Conf: conf,
			Tok:  tok,
		}
		if err = box.SaveCredentials(h.storage.b, cred); err != nil {
			h.errFn(log.Ctx{"err": err.Error()})("Unable to save C2S credentials for logged actor")
		}

		if strings.ToLower(provider) != "local" {
			h.v.addFlashMessage(Success, w, r, fmt.Sprintf("Login successful with %s", provider))
		} else {
			h.v.addFlashMessage(Success, w, r, "Login successful")
		}
	}
	if err := h.v.saveAccountToSession(w, r, acct); err != nil {
		h.errFn()("Unable to save account to session")
	}
	h.v.Redirect(w, r, "/", http.StatusFound)
}

func (h *handler) ShowPublicKey(w http.ResponseWriter, r *http.Request) {
	authors := ContextAuthors(r.Context())
	if len(authors) != 1 {
		m := &errorModel{
			Status:     http.StatusNotFound,
			StatusText: "Account not found",
			Title:      "Not found",
		}
		if err := h.v.RenderTemplate(r, w, m.Template(), m); err != nil {
			h.v.HandleErrors(w, r, err)
		}
		return
	}
	account, _ := authors.First()
	k := account.Metadata.Key.Public

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Write(k)
}

// HandleShow serves most of the GET requests
func (h *handler) HandleShow(w http.ResponseWriter, r *http.Request) {
	m := ContextModel(r.Context())
	if m == nil {
		m = &errorModel{
			Status:     http.StatusInternalServerError,
			StatusText: "Unable to load the page.",
			Title:      "Oops!!",
		}
	}
	if cursor := ContextCursor(r.Context()); cursor != nil {
		if mod, ok := m.(CursorSetter); ok {
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
