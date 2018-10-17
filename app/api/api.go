package api

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/mariusor/littr.go/app"
	localap "github.com/mariusor/littr.go/app/activitypub"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/frontend"
	"github.com/spacemonkeygo/httpsig"

	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	as "github.com/mariusor/activitypub.go/activitystreams"
	j "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

const (
	MaxContentItems = 200
)

var Logger log.FieldLogger

var BaseURL string
var AccountsURL string
var OutboxURL string

const NotFound = 404
const InternalError = 500

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Fields []Field

type Error struct {
	Code  int
	Error error
}

func init() {
	host := os.Getenv("HOSTNAME")

	if app.Instance.Secure {
		BaseURL = fmt.Sprintf("https://%s/api", host)
	} else {
		BaseURL = fmt.Sprintf("http://%s/api", host)
	}
	Config.BaseUrl = BaseURL

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	AccountsURL = BaseURL + "/accounts"
	OutboxURL = BaseURL + "/outbox"
	if Logger == nil {
		Logger = log.StandardLogger()
	}
}

func Errorf(c int, m string, args ...interface{}) *Error {
	return &Error{c, errors.Errorf(m, args...)}
}

func GetContext() j.Context {
	return j.Context{
		{IRI: j.IRI(as.ActivityBaseURI)},
		{IRI: j.IRI("https://w3id.org/security/v1")},
		{j.Term("score"), j.IRI(fmt.Sprintf("%s/ns/#score", app.Instance.BaseURL))},
	}
}

func BuildGlobalOutboxID() as.ObjectID {
	return as.ObjectID(fmt.Sprintf("%s/outbox", BaseURL))
}

func BuildActorID(a models.Account) as.ObjectID {
	return as.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, url.PathEscape(a.Hash.String())))
}

func BuildActorHashID(a models.Account) as.ObjectID {
	return as.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, url.PathEscape(a.Hash.String())))
}

func BuildCollectionID(a models.Account, o as.Item) as.ObjectID {
	if len(a.Handle) > 0 {
		return as.ObjectID(fmt.Sprintf("%s/%s/%s", AccountsURL, url.PathEscape(a.Hash.String()), getObjectType(o)))
	}
	return as.ObjectID(fmt.Sprintf("%s/%s", BaseURL, getObjectType(o)))
}

func BuildRepliesCollectionID(i as.Item) as.ObjectID {
	return as.ObjectID(fmt.Sprintf("%s/replies", *i.GetID()))
}

func BuildObjectIDFromItem(i models.Item) (as.ObjectID, bool) {
	if len(i.Hash) == 0 {
		return as.ObjectID(""), false
	}
	if i.SubmittedBy != nil {
		hash := i.SubmittedBy.Hash
		return as.ObjectID(fmt.Sprintf("%s/%s/outbox/%s/object", AccountsURL, url.PathEscape(hash.String()), url.PathEscape(i.Hash.String()))), true
	} else {
		return as.ObjectID(fmt.Sprintf("%s/outbox/%s/object", BaseURL, url.PathEscape(i.Hash.String()))), true
	}
}

func BuildObjectIDFromVote(v models.Vote) as.ObjectID {
	att := "liked"
	//if v.Weight < 0 {
	//	att = "disliked"
	//}
	return as.ObjectID(fmt.Sprintf("%s/%s/%s/%s", AccountsURL, url.PathEscape(v.SubmittedBy.Handle), att, url.PathEscape(v.Item.Hash.String())))
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

func HandleError(w http.ResponseWriter, r *http.Request, code int, errs ...error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", " no-store, must-revalidate")
	w.Header().Set("Pragma", " no-cache")
	w.Header().Set("Expires", " 0")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)

	type error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	type eresp struct {
		Status int     `json:"status"`
		Errors []error `json:"errors"`
	}

	res := eresp{
		Status: code,
		Errors: []error{},
	}
	for _, err := range errs {
		e := error{
			Message: err.Error(),
		}
		Logger.WithFields(log.Fields{}).Error(err)
		res.Errors = append(res.Errors, e)
	}

	j, _ := json.Marshal(res)
	w.Write(j)
}

type keyLoader struct {
	acc models.Account
}

func (k *keyLoader) GetKey(id string) interface{} {
	// keyId="http://littr.git/api/accounts/e33c4ff5#main-key"
	var err error

	u, err := url.Parse(id)
	if err != nil {
		return err
	}
	if u.Fragment != "main-key" {
		// invalid generated public key id
		return errors.Errorf("invalid key")
	}
	hash := path.Base(u.Path)
	k.acc, err = db.Config.LoadAccount(models.LoadAccountsFilter{Key: []string{hash}})
	if err != nil {
		return err
	}

	var pub crypto.PublicKey
	pub, err = x509.ParsePKIXPublicKey(k.acc.Metadata.Key.Public)
	if err != nil {
		return err
	}
	return pub
}

func VerifyHttpSignature(next http.Handler) http.Handler {
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
				w.Header()["WWW-Authenticate"] = []string{challenge}
				Logger.WithFields(log.Fields{
					"handle": acct.Handle,
					"hash":   acct.Hash,
					"header": fmt.Sprintf("%q", r.Header),
				}).Warningf("invalid www signature")
				// TODO(marius): here we need to implement some outside logic, as to we want to allow non-signed
				//   requests on some urls, but not on others - probably another handler to check for Anonymous
				//   would suffice.
				//HandleError(w, r, http.StatusUnauthorized, err)
				//return
			} else {
				acct = &getter.acc
				Logger.WithFields(log.Fields{
					"handle": acct.Handle,
					"hash":   acct.Hash,
				}).Debugf("loaded account from http signature header saved to context")
			}
		}
		ctx := context.WithValue(r.Context(), models.AccountCtxtKey, acct)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
	return http.HandlerFunc(fn)
}
