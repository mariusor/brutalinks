package api

import (
	"crypto"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/frontend"
	"github.com/spacemonkeygo/httpsig"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
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

var CurrentAccount *models.Account

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
	https := os.Getenv("HTTPS") != ""
	host := os.Getenv("HOSTNAME")

	if https {
		BaseURL = fmt.Sprintf("https://%s/api", host)
	} else {
		BaseURL = fmt.Sprintf("http://%s/api", host)
	}

	AccountsURL = BaseURL + "/accounts"
	OutboxURL = BaseURL + "/outbox"
	if CurrentAccount == nil {
		CurrentAccount = frontend.AnonymousAccount()
	}
	CurrentAccount.Metadata = nil
	Logger = log.StandardLogger()
}

func getHashFromAP(obj ap.ObjectOrLink) models.Hash {
	var h models.Hash
	if obj.IsLink() {
		h = models.Hash(path.Base(string(obj.(ap.IRI))))
	} else {
		h = getHash(obj.GetID())
	}
	return h
}

func getHash(i *ap.ObjectID) models.Hash {
	if i == nil {
		return ""
	}
	s := strings.Split(string(*i), "/")
	return models.Hash(s[len(s)-1])
}

func getAccountHandle(o ap.ObjectOrLink) string {
	if o == nil {
		return ""
	}
	i := o.(ap.IRI)
	s := strings.Split(string(i), "/")
	return s[len(s)-1]
}

func Errorf(c int, m string, args ...interface{}) *Error {
	return &Error{c, errors.Errorf(m, args...)}
}

func GetContext() j.Context {
	return j.Context{
		{IRI: j.IRI(ap.ActivityBaseURI)},
		{IRI: j.IRI("https://w3id.org/security/v1")},
		{j.Term("score"), j.IRI(fmt.Sprintf("%sns/#score", app.Instance.BaseUrl()))},
	}
}

func BuildActorID(a models.Account) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, url.PathEscape(a.Handle)))
}
func BuildActorHashID(a models.Account) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, url.PathEscape(a.Hash.String())))
}

func BuildCollectionID(a models.Account, o ap.Item) ap.ObjectID {
	if len(a.Handle) > 0 {
		return ap.ObjectID(fmt.Sprintf("%s/%s/%s", AccountsURL, url.PathEscape(a.Handle), getObjectType(o)))
	}
	return ap.ObjectID(fmt.Sprintf("%s/%s", BaseURL, getObjectType(o)))
}

func BuildRepliesCollectionID(i ap.Item) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/replies", *i.GetID()))
}

func BuildObjectIDFromItem(i models.Item) (ap.ObjectID, bool) {
	if len(i.Hash) == 0 {
		return ap.ObjectID(""), false
	}
	if i.SubmittedBy != nil {
		handle := i.SubmittedBy.Handle
		return ap.ObjectID(fmt.Sprintf("%s/%s/outbox/%s", AccountsURL, url.PathEscape(handle), url.PathEscape(i.Hash.String()))), true
	} else {
		return ap.ObjectID(fmt.Sprintf("%s/outbox/%s", BaseURL, url.PathEscape(i.Hash.String()))), true
	}
}

func BuildObjectIDFromVote(v models.Vote) ap.ObjectID {
	att := "liked"
	//if v.Weight < 0 {
	//	att = "disliked"
	//}
	return ap.ObjectID(fmt.Sprintf("%s/%s/%s/%s", AccountsURL, url.PathEscape(v.SubmittedBy.Handle), att, url.PathEscape(v.Item.Hash.String())))
}

func getObjectType(el ap.Item) string {
	if el == nil {
		return ""
	}
	var (
		label               = ""
		typeOutbox          = reflect.TypeOf(ap.Outbox{})
		typeOutboxStream    = reflect.TypeOf(ap.OutboxStream{})
		typeInbox           = reflect.TypeOf(ap.Inbox{})
		typeInboxStream     = reflect.TypeOf(ap.InboxStream{})
		typeLiked           = reflect.TypeOf(ap.Liked{})
		typeLikedCollection = reflect.TypeOf(ap.LikedCollection{})
		typePerson          = reflect.TypeOf(ap.Person{})
		typeLocalPerson     = reflect.TypeOf(Person{})
	)
	typ := reflect.TypeOf(el)
	val := reflect.ValueOf(el)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		val = val.Elem()
	}
	switch typ {
	case typeOutbox:
		fallthrough
	case typeOutboxStream:
		label = "outbox"
	case typeInbox:
		fallthrough
	case typeInboxStream:
		label = "inbox"
	case typeLiked:
		fallthrough
	case typeLikedCollection:
		label = "liked"
	case typePerson:
		o := val.Interface().(ap.Person)
		for _, n := range o.Name {
			label = n.Value
			break
		}
	case typeLocalPerson:
		o := val.Interface().(Person)
		for _, n := range o.Name {
			label = n.Value
			break
		}
	}
	return label
}

func HandleError(w http.ResponseWriter, r *http.Request, code int, errs ...error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
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
	w.Header().Del("Cookie")
	w.Write(j)
}

type keyLoader struct {
	acc models.Account
}

func (k keyLoader) GetKey(id string) interface{} {
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
	v := httpsig.NewVerifier(getter)
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
		if r.Header["Authorization"] == nil {
			CurrentAccount = frontend.AnonymousAccount()
		} else {
			// only verify http-signature if present
			err := v.Verify(r)
			if err != nil {
				w.Header()["WWW-Authenticate"] = []string{challenge}
				HandleError(w, r, http.StatusUnauthorized, err)
				return
			} else {
				CurrentAccount = &getter.acc
				Logger.WithFields(log.Fields{
					"handle": CurrentAccount.Handle,
					"hash":   CurrentAccount.Hash,
					"email":  CurrentAccount.Email,
				}).Infof("loaded account from http signature header ")
			}
		}
		next.ServeHTTP(w, r)
	})
	return http.HandlerFunc(fn)
}

func ShowHeaders(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		for name, val := range r.Header {
			Logger.Infof("%s: %s", name, val)
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
