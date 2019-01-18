package api

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/mariusor/littr.go/app"
	localap "github.com/mariusor/littr.go/app/activitypub"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/frontend"
	"github.com/mariusor/littr.go/app/log"
	"github.com/spacemonkeygo/httpsig"

	"github.com/juju/errors"
	ap "github.com/go-ap/activitypub.go/activitypub"
	as "github.com/go-ap/activitypub.go/activitystreams"
	j "github.com/go-ap/activitypub.go/jsonld"
)

const (
	MaxContentItems = 50
)

type InternalError struct {
}

type UserError struct {
}

type handler struct{
	repo *repository
	logger log.Logger
}

type Config struct {
	Logger log.Logger
	BaseURL string
}

func Init(c Config) handler {
	BaseURL = c.BaseURL
	ActorsURL = c.BaseURL + "/actors"
	h := handler{
		logger: c.Logger,
	}
	h.repo = New(c)
	return h
}

var BaseURL string
var ActorsURL string

const NotFoundStatus = 404
const InternalErrorStatus = 500

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Fields []Field

type Error struct {
	Code  int
	Error error
}

func Errorf(c int, m string, args ...interface{}) *Error {
	return &Error{c, errors.Errorf(m, args...)}
}

func GetContext() j.Context {
	return j.Context{
		{IRI: j.IRI(as.ActivityBaseURI)},
		{IRI: j.IRI("https://w3id.org/security/v1")},
		{j.Term("score"), j.IRI(fmt.Sprintf("%s/ns#score", app.Instance.BaseURL))},
	}
}

func BuildGlobalOutboxID() as.ObjectID {
	return as.ObjectID(fmt.Sprintf("%s/self/outbox", BaseURL))
}

func BuildActorID(a app.Account) as.ObjectID {
	return as.ObjectID(fmt.Sprintf("%s/%s", ActorsURL, url.PathEscape(a.Hash.String())))
}

func BuildCollectionID(a app.Account, o as.Item) as.ObjectID {
	if len(a.Handle) > 0 {
		return as.ObjectID(fmt.Sprintf("%s/%s/%s", ActorsURL, url.PathEscape(a.Hash.String()), getObjectType(o)))
	}
	return as.ObjectID(fmt.Sprintf("%s/%s", BaseURL, getObjectType(o)))
}

func BuildRepliesCollectionID(i as.Item) as.ObjectID {
	return as.ObjectID(fmt.Sprintf("%s/replies", *i.GetID()))
}

func BuildObjectIDFromItem(i app.Item) (as.ObjectID, bool) {
	if len(i.Hash) == 0 {
		return as.ObjectID(""), false
	}
	if i.SubmittedBy != nil {
		hash := i.SubmittedBy.Hash
		return as.ObjectID(fmt.Sprintf("%s/%s/outbox/%s/object", ActorsURL, url.PathEscape(hash.String()), url.PathEscape(i.Hash.String()))), true
	} else {
		return as.ObjectID(fmt.Sprintf("%s/self/outbox/%s/object", BaseURL, url.PathEscape(i.Hash.String()))), true
	}
}

func BuildObjectIDFromVote(v app.Vote) as.ObjectID {
	att := "liked"
	return as.ObjectID(fmt.Sprintf("%s/%s/%s/%s", ActorsURL, url.PathEscape(v.SubmittedBy.Handle), att, url.PathEscape(v.Item.Hash.String())))
}

func getObjectType(el as.Item) string {
	if el == nil {
		return ""
	}
	var label = ""
	switch el.(type) {
	case *ap.Outbox:
		label = "outbox"
	case ap.Outbox:
		label = "outbox"
	case *ap.Inbox:
		label = "inbox"
	case ap.Inbox:
		label = "inbox"
	case ap.Liked:
		label = "liked"
	case *ap.Liked:
		label = "liked"
	case ap.Followers:
		label = "followers"
	case *ap.Followers:
		label = "followers"
	case ap.Following:
		label = "following"
	case *ap.Following:
		label = "following"
	case as.Person:
		o := el.(as.Person)
		for _, n := range o.Name {
			label = n.Value
			break
		}
	case *as.Person:
		o := el.(*as.Person)
		for _, n := range o.Name {
			label = n.Value
			break
		}
	case localap.Person:
		o := el.(localap.Person)
		for _, n := range o.Name {
			label = n.Value
			break
		}
	case *localap.Person:
		o := el.(*localap.Person)
		for _, n := range o.Name {
			label = n.Value
			break
		}
	}
	return label
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
		return http.StatusNotAcceptable
	}
	if errors.IsMethodNotAllowed(e) {
		return http.StatusMethodNotAllowed
	}
	return http.StatusInternalServerError
}

func (h handler)HandleError(w http.ResponseWriter, r *http.Request, errs ...error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	type error struct {
		Code    int      `json:"code,omitempty"`
		Message string   `json:"message"`
		Trace   []string `json:"trace,omitempty"`
	}
	type eresp struct {
		Status int     `json:"status,omitempty"`
		Errors []error `json:"errors"`
	}

	res := eresp {
		Errors: []error{},
	}

	code := http.StatusInternalServerError
	for _, err := range errs {
		if err == nil {
			continue
		}
		var msg string
		var trace []string
		switch e := err.(type) {
		case *json.UnmarshalTypeError:
			msg = fmt.Sprintf("UnmarshalTypeError: Value[%s] Type[%v]\n", e.Value, e.Type)
		case *json.InvalidUnmarshalError:
			msg = fmt.Sprintf("InvalidUnmarshalError: Type[%v]\n", e.Type)
		case *errors.Err:
			msg = fmt.Sprintf("%v", e)
			if app.Instance.Config.Env == app.DEV {
				trace = e.StackTrace()
			}
		default:
			msg = e.Error()
		}
		e := error{
			Message: msg,
			Trace:   trace,
		}
		res.Errors = append(res.Errors, e)
		code = httpErrorResponse(err)
	}
	res.Status = code

	j, _ := json.Marshal(res)
	w.WriteHeader(code)
	w.Write(j)
}

type keyLoader struct {
	acc app.Account
}

func loadFederatedActor(id as.IRI) (as.Actor, error) {
	return as.Actor{}, errors.NotImplementedf("federated actors loading is not implemented")
}

func (k *keyLoader) GetKey(id string) interface{} {
	// keyId="http://littr.git/api/actors/e33c4ff5#main-key"
	var err error

	u, err := url.Parse(id)
	if err != nil {
		return err
	}
	if u.Fragment != "main-key" {
		// invalid generated public key id
		return errors.Errorf("invalid key")
	}

	if err := validateLocalIRI(as.IRI(id)); err == nil {
		hash := path.Base(u.Path)
		k.acc, err = db.Config.LoadAccount(app.LoadAccountsFilter{Key: app.Hashes{app.Hash(hash)}})
		if err != nil {
			return errors.Annotatef(err, "unable to find local account matching key id %s", id)
		}
	} else {
		// @todo(queue_support): this needs to be moved to using queues
		actor, err := loadFederatedActor(as.IRI(u.RequestURI()))
		if err != nil {
			return errors.Annotatef(err, "unable to load federated account matching key id %s", id)
		}
		k.acc.FromActivityPub(actor)
	}

	var pub crypto.PublicKey
	pub, err = x509.ParsePKIXPublicKey(k.acc.Metadata.Key.Public)
	if err != nil {
		return err
	}
	return pub
}

func (h handler)VerifyHttpSignature(next http.Handler) http.Handler {
	getter := keyLoader{}

	realm := app.Instance.HostName
	v := httpsig.NewVerifier(&getter)
	v.SetRequiredHeaders([]string{"(request-target)", "host", "date"})

	var challengeParams []string
	if realm != "" {
		challengeParams = append(challengeParams, fmt.Sprintf("realm=%q", realm))
	}
	if headers := v.RequiredHeaders(); len(headers) > 0 {
		challengeParams = append(challengeParams, fmt.Sprintf("headers=%q", strings.Join(headers, " ")))
	}

	challenge := "Signature"
	if len(challengeParams) > 0 {
		challenge += fmt.Sprintf(" %s", strings.Join(challengeParams, ", "))
	}

	fn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var acct = frontend.AnonymousAccount()
		if r.Header["Authorization"] != nil {
			// only verify http-signature if present
			if err := v.Verify(r); err != nil {
				w.Header().Add("WWW-Authenticate", challenge)
				h.logger.WithContext(log.Ctx{
					"handle": acct.Handle,
					"hash":   acct.Hash,
					//"auth": fmt.Sprintf("%v", r.Header["Authorization"]),
					"req": fmt.Sprintf("%s:%s", r.Method, r.URL.RequestURI()),
					"err": err,
				}).Warn("invalid HTTP signature")
				// TODO(marius): here we need to implement some outside logic, as to we want to allow non-signed
				//   requests on some urls, but not on others - probably another handler to check for Anonymous
				//   would suffice.
				//HandleError(w, r, http.StatusUnauthorized, err)
				//return
			} else {
				acct = getter.acc
				h.logger.WithContext(log.Ctx{
					"handle": acct.Handle,
					"hash":   acct.Hash,
				}).Debug("loaded account from HTTP signature header")
			}
		}
		ctx := context.WithValue(r.Context(), app.AccountCtxtKey, acct)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
	return http.HandlerFunc(fn)
}
