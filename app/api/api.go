package api

import (
	"encoding/json"
	"net/http"
	"path"

	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/buger/jsonparser"
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

	Logger = log.StandardLogger()
}

type PublicKey struct {
	Id           ap.ObjectID     `jsonld:"id,omitempty"`
	Owner        ap.ObjectOrLink `jsonld:"owner,omitempty"`
	PublicKeyPem string          `jsonld:"publicKeyPem,omitempty"`
}

// Person it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/actors.go#Actor
// We need it here in order to be able to add to it our Score property
type Person struct {
	ap.Person
	PublicKey PublicKey `jsonld:"publicKey,omitempty"`
	// Score is our own custom property for which we needed to extend the existing AP one
	Score int64 `jsonld:"score"`
}

func (p Person) GetID() *ap.ObjectID {
	id := ap.ObjectID(p.ID)
	return &id
}
func (p Person) GetType() ap.ActivityVocabularyType {
	return ap.ActivityVocabularyType(p.Type)
}
func (p Person) IsLink() bool {
	return false
}
func (p Person) IsObject() bool {
	return true
}

// Article it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/objects.go#Object
// We need it here in order to be able to add to it our Score property
type Article struct {
	ap.Object
	Score int64 `jsonld:"score"`
}

func (a Article) GetID() *ap.ObjectID {
	id := ap.ObjectID(a.ID)
	return &id
}
func (a Article) GetType() ap.ActivityVocabularyType {
	return ap.ActivityVocabularyType(a.Type)
}
func (a Article) IsLink() bool {
	return false
}
func (a Article) IsObject() bool {
	return true
}

func (a *Article) UnmarshalJSON(data []byte) error {
	it := ap.Object{}
	err := it.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	a.Object = it
	if score, err := jsonparser.GetInt(data, "score"); err == nil {
		a.Score = score
	}

	return nil
}

func (p *Person) UnmarshalJSON(data []byte) error {
	app := ap.Person{}
	err := app.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	p.Person = app
	if score, err := jsonparser.GetInt(data, "score"); err == nil {
		p.Score = score
	}

	return nil
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
		{j.Term("score"), j.IRI("http://littr.me/as#score")},
	}
}

func BuildActorID(a models.Account) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, url.PathEscape(a.Handle)))
}
func BuildActorHashID(a models.Account) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, url.PathEscape(a.Hash.String())))
}

func BuildCollectionID(a models.Account, o ap.CollectionInterface) ap.ObjectID {
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

func BuildObjectURL(b ap.LinkOrURI, el ap.Item) ap.URI {
	if el == nil {
		return ""
	}
	pURL := ap.URI(BaseURL)
	if b != nil && b.GetLink() != "" {
		pURL = b.GetLink()
	}

	return ap.URI(fmt.Sprintf("%s/%s", pURL, getObjectType(el)))
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
	w.Write(j)
}
