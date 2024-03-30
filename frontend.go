package brutalinks

import (
	"context"
	"embed"
	"fmt"
	"net/http"

	"git.sr.ht/~mariusor/brutalinks/internal/config"
	log "git.sr.ht/~mariusor/lw"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/gorilla/csrf"
)

const (
	sessionName = "_s"
	csrfName    = "_c"
)

//go:embed templates
var templateFs embed.FS

type handler struct {
	conf    appConfig
	v       *view
	storage *repository
	logger  log.Logger
}

func (h handler) infoFn(ctx ...log.Ctx) LogFn {
	if h.logger == nil {
		return defaultLogFn
	}
	return h.logger.WithContext(ctx...).Infof
}
func (h handler) errFn(ctx ...log.Ctx) LogFn {
	if h.logger == nil {
		return defaultLogFn
	}
	return h.logger.WithContext(ctx...).Errorf
}

type appConfig struct {
	config.Configuration
	BaseURL string
	Logger  log.Logger
}

var defaultLogFn = func(string, ...interface{}) {}
var defaultCtxLogFn = func(c ...log.Ctx) LogFn { return defaultLogFn }

const fedboxProvider = "fedbox"

func (h *handler) init(c appConfig) error {
	var err error

	if c.Logger != nil {
		h.logger = c.Logger.WithContext(log.Ctx{"log": "handlers"})
	}

	h.conf = c

	if err := ConnectFedBOX(h, h.conf); err != nil {
		return errors.Annotatef(err, "error connecting to ActivityPub service: %s", h.conf.APIURL)
	}
	if h.v, err = ViewInit(h.conf, h.logger); err != nil {
		return errors.Annotatef(err, "error initializing view")
	}
	return nil
}

func ConnectFedBOX(h *handler, c appConfig) error {
	var err error
	h.storage, err = ActivityPubService(c)
	if err != nil {
		return fmt.Errorf("failed to load actor: %w", err)
	}
	return nil
}

func AuthorizeOAuthClient(storage *repository, c appConfig) (*Account, error) {
	config := c.GetOauth2Config(fedboxProvider, c.BaseURL)
	if len(config.ClientID) == 0 {
		return nil, errors.Newf("invalid OAuth2 configuration")
	}
	appURL := vocab.IRI(config.ClientID)
	if u, err := appURL.URL(); err != nil || u.Hostname() == "" {
		appURL = actors.IRI(storage.BaseURL()).AddPath(config.ClientID)
	}
	oauth, err := storage.fedbox.Actor(context.TODO(), appURL)
	if err != nil {
		return nil, err
	}
	if oauth == nil {
		return nil, errors.Newf("unable to load OAuth2 client application actor")
	}
	app := new(Account)
	app.FromActivityPub(oauth)

	tok, err := config.PasswordCredentialsToken(context.TODO(), config.ClientID, config.ClientSecret)
	if err != nil {
		return app, err
	} else {
		if tok == nil {
			return app, errors.Newf("Failed to load a valid OAuth2 token for client")
		}
		app.Metadata.OAuth.Provider = fedboxProvider
		app.Metadata.OAuth.Token = tok
	}
	return app, nil
}

func loggedAccount(r *http.Request) *Account {
	if acct := ContextAccount(r.Context()); acct != nil {
		return acct
	}
	return &AnonymousAccount
}

func isInverted(r *http.Request) bool {
	cookies := r.Cookies()
	for _, c := range cookies {
		if c.Name == "inverted" {
			return true
		}
	}
	return false
}

func (v *view) saveAccountToSession(w http.ResponseWriter, r *http.Request, a *Account) error {
	if !v.s.enabled || w == nil || r == nil {
		return nil
	}
	s, err := v.s.get(w, r)
	if err != nil {
		return err
	}
	s.Values[SessionUserKey] = *a
	return nil
}

func (v *view) loadCurrentAccountFromSession(w http.ResponseWriter, r *http.Request) (*Account, error) {
	acc := AnonymousAccount
	if !v.s.enabled || w == nil || r == nil {
		return &acc, nil
	}
	s, err := v.s.get(w, r)
	if err != nil {
		v.s.clear(w, r)
		return &acc, errors.Annotatef(err, "session load error")
	}
	// load the current account from the session or setting it to anonymous
	if raw, ok := s.Values[SessionUserKey]; ok {
		if acc, ok = raw.(Account); !ok {
			v.errFn(log.Ctx{"sess": s.Values})("invalid account in session")
		}
	}
	if !acc.IsLogged() {
		return &acc, nil
	}

	lCtx := log.Ctx{
		"handle": acc.Handle,
		"hash":   acc.Hash,
	}
	var f *Filters
	if acc.IsLogged() && acc.HasMetadata() {
		f = new(Filters)
		f.IRI = CompStrs{EqualsString(acc.Metadata.ID)}
		lCtx["iri"] = acc.Metadata.ID
	} else {
		f = FilterAccountByHandle(acc.Handle)
	}
	repo := ContextRepository(r.Context())
	a, err := repo.account(r.Context(), f)
	if err != nil {
		return &acc, errors.Annotatef(err, "unable to load actor for session account")
	}
	loadAccountData(&acc, *a)
	v.infoFn(lCtx)("loaded account from session")
	return &acc, nil
}

func (v *view) SetSecurityHeaders(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if conf := v.c; conf.Secure {
			if conf.Env.IsProd() {
				w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			} else {
				w.Header().Set("Strict-Transport-Security", "max-age=0")
			}
		}
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Xss-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		v.SetCSP(ContextModel(r.Context()), w)
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// loadAccountData is used so we don't stomp over the values already stored in the session's account
func loadAccountData(a *Account, b Account) {
	if a.Hash != b.Hash {
		return
	}
	if len(a.Handle) == 0 && len(b.Handle) > 0 {
		a.Handle = b.Handle
	}
	if a.CreatedAt.IsZero() && !b.CreatedAt.IsZero() {
		a.CreatedAt = b.CreatedAt
	}
	if a.CreatedBy == nil && b.CreatedBy != nil {
		a.CreatedBy = b.CreatedBy
	}
	if a.UpdatedAt.IsZero() && !b.UpdatedAt.IsZero() {
		a.UpdatedAt = b.UpdatedAt
	}
	if a.Metadata == nil && b.Metadata != nil {
		a.Metadata = b.Metadata
	}
	if a.Metadata != nil && b.Metadata != nil {
		if len(a.Metadata.ID) == 0 && len(b.Metadata.ID) > 0 {
			a.Metadata.ID = b.Metadata.ID
		}
		if a.Metadata.Key == nil && b.Metadata.Key != nil {
			a.Metadata.Key = b.Metadata.Key
		}
		if a.Metadata.OutboxUpdated.IsZero() && !b.Metadata.OutboxUpdated.IsZero() {
			a.Metadata.OutboxUpdated = b.Metadata.OutboxUpdated
		}
		if len(b.Metadata.Outbox) > 0 {
			for _, ob := range b.Metadata.Outbox {
				if !a.Metadata.Outbox.Contains(ob) {
					a.Metadata.Outbox = append(a.Metadata.Outbox, ob)
				}
			}
		}
		if len(b.Metadata.Tags) > 0 {
			for _, tt := range b.Metadata.Tags {
				if !a.Metadata.Tags.Contains(tt) {
					a.Metadata.Tags = append(a.Metadata.Tags, tt)
				}
			}
		}
		if len(a.Metadata.Name) == 0 && len(b.Metadata.Name) > 0 {
			a.Metadata.Name = b.Metadata.Name
		}
		if len(a.Metadata.Blurb) == 0 && len(b.Metadata.Blurb) > 0 {
			a.Metadata.Blurb = b.Metadata.Blurb
		}
		if len(a.Metadata.AuthorizationEndPoint) == 0 && len(b.Metadata.AuthorizationEndPoint) > 0 {
			a.Metadata.AuthorizationEndPoint = b.Metadata.AuthorizationEndPoint
		}
		if len(a.Metadata.FollowersIRI) == 0 && len(b.Metadata.FollowersIRI) > 0 {
			a.Metadata.FollowersIRI = b.Metadata.FollowersIRI
		}
		if len(a.Metadata.FollowingIRI) == 0 && len(b.Metadata.FollowingIRI) > 0 {
			a.Metadata.FollowingIRI = b.Metadata.FollowingIRI
		}
		if len(a.Metadata.InboxIRI) == 0 && len(b.Metadata.InboxIRI) > 0 {
			a.Metadata.InboxIRI = b.Metadata.InboxIRI
		}
		if len(a.Metadata.OutboxIRI) == 0 && len(b.Metadata.OutboxIRI) > 0 {
			a.Metadata.OutboxIRI = b.Metadata.OutboxIRI
		}
		if len(a.Metadata.LikedIRI) == 0 && len(b.Metadata.LikedIRI) > 0 {
			a.Metadata.LikedIRI = b.Metadata.LikedIRI
		}
		if len(a.Metadata.URL) == 0 && len(b.Metadata.URL) > 0 {
			a.Metadata.URL = b.Metadata.URL
		}
		if a.Metadata.OAuth.Token == nil && b.Metadata.OAuth.Token != nil {
			a.Metadata.OAuth.Token = b.Metadata.OAuth.Token
		}
		if len(a.Metadata.OAuth.Code) == 0 && len(b.Metadata.OAuth.Code) > 0 {
			a.Metadata.OAuth.Code = b.Metadata.OAuth.Code
		}
		if len(a.Metadata.OAuth.State) == 0 && len(b.Metadata.OAuth.State) > 0 {
			a.Metadata.OAuth.State = b.Metadata.OAuth.State
		}
		if len(a.Metadata.OAuth.Provider) == 0 && len(b.Metadata.OAuth.Provider) > 0 {
			a.Metadata.OAuth.Provider = b.Metadata.OAuth.Provider
		}
	}
	if a.Pub == nil && b.Pub != nil {
		a.Pub = b.Pub
	}
	if len(a.Followers) == 0 && len(b.Followers) > 0 {
		a.Followers = b.Followers
	}
	if len(a.Following) == 0 && len(b.Following) > 0 {
		a.Following = b.Following
	}
	if len(a.Blocked) == 0 && len(b.Blocked) > 0 {
		a.Blocked = b.Blocked
	}
	if len(a.Ignored) == 0 && len(b.Ignored) > 0 {
		a.Ignored = b.Ignored
	}
	if a.Parent == nil && b.Parent != nil {
		a.Parent = b.Parent
	}
	if len(a.children) == 0 && len(b.children) > 0 {
		a.children = b.children
	}
	if a.Flags == 0 && b.Flags > 0 {
		a.Flags = b.Flags
	}
}

func (v *view) LoadSession(next http.Handler) http.Handler {
	if !v.s.enabled {
		return next
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		var (
			storage      = ContextRepository(r.Context())
			clearSession bool
			err          error
			ltx          log.Ctx
		)
		acc, err := v.loadCurrentAccountFromSession(w, r)
		if err != nil {
			acc = &AnonymousAccount
			v.errFn(log.Ctx{"err": err.Error()})("unable to load actor from session")
		}
		if acc.IsLogged() {
			v.infoFn(log.Ctx{"handle": acc.Handle})("Setting FedBOX logged account")
			defer func() {
				v.infoFn()("Unsetting FedBOX logged account")
				storage.WithAccount(&AnonymousAccount)
			}()
			ctx := context.WithValue(r.Context(), LoggedAccountCtxtKey, acc)
			if err = storage.LoadAccountDetails(ctx, acc); err != nil {
				clearSession = true
				v.errFn(ltx, log.Ctx{"err": err.Error()})("unable to load account")
			} else {
				storage.WithAccount(acc)
			}
		}
		if clearSession {
			v.s.clear(w, r)
		}
		r = r.WithContext(context.WithValue(r.Context(), LoggedAccountCtxtKey, acc))
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func (h handler) NeedsSessions(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.v.s.enabled {
			h.v.HandleErrors(w, r, errors.NotFoundf("sessions are disabled"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// HandleAbout serves /about request
// It's something Mastodon compatible servers should show
func (h *handler) HandleAbout(w http.ResponseWriter, r *http.Request) {
	m := &aboutModel{Title: "About"}

	repo := h.storage
	info, err := repo.LoadInfo()
	if err != nil {
		h.logger.WithContext(log.Ctx{"err": err}).Errorf("unable to load service actor for FedBOX")
	}
	m.Desc.Description = info.Description

	h.v.RenderTemplate(r, w, m.Template(), m)
}

func httpErrorResponse(e error) int {
	if errors.IsBadRequest(e) {
		return http.StatusBadRequest
	}
	if errors.IsForbidden(e) {
		return http.StatusForbidden
	}
	if errors.IsNotSupported(e) {
		return http.StatusHTTPVersionNotSupported
	}
	if errors.IsMethodNotAllowed(e) {
		return http.StatusMethodNotAllowed
	}
	if errors.IsNotFound(e) {
		return http.StatusNotFound
	}
	if errors.IsNotImplemented(e) {
		return http.StatusNotImplemented
	}
	if errors.IsUnauthorized(e) {
		return http.StatusUnauthorized
	}
	if errors.IsTimeout(e) {
		return http.StatusGatewayTimeout
	}
	if errors.IsNotValid(e) {
		return http.StatusInternalServerError
	}
	return http.StatusInternalServerError
}

var defaultSessionKey = []byte{0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1}

func (h *handler) ErrorHandler(errs ...error) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		h.v.HandleErrors(w, r, errs...)
	}
	return http.HandlerFunc(fn)
}

func (h handler) CSRF(next http.Handler) http.Handler {
	opts := []csrf.Option{
		csrf.CookieName(csrfName),
		csrf.FieldName(csrfName),
		csrf.Secure(h.conf.Env.IsProd()),
		csrf.ErrorHandler(h.ErrorHandler(errors.Forbiddenf("Invalid request token"))),
	}
	var authKey []byte
	if len(h.conf.SessionKeys) > 0 {
		authKey = h.conf.SessionKeys[0]
	} else {
		if h.conf.Env.IsProd() {
			h.errFn()("Invalid CSRF auth key")
		}
		// TODO(marius): WTF is this?
		authKey = defaultSessionKey
	}
	return csrf.Protect(authKey, opts...)(next)
}
