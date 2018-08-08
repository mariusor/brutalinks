package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"fmt"
	"github.com/go-chi/chi"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	j "github.com/mariusor/activitypub.go/jsonld"
	log "github.com/sirupsen/logrus"
	"os"
	"reflect"
	"strings"
	"github.com/mariusor/littr.go/models"
	"net/url"
)

var Db *sql.DB
var BaseURL string
var AccountsURL string

const NotFound = 404
const InternalError = 500

type _obj ap.Object
type _per ap.Person

// Person is an extension of the AP/Person object
type Person struct {
	_per
	Score int64	`jsonld:"score"`
}

// Article is an extension of the AP/Article object
type Article struct {
	_obj
	Score int64	`jsonld:"score"`
}

func (a Article)GetID() *ap.ObjectID{
	return &a._obj.ID
}
func (a Article)GetType() ap.ActivityVocabularyType{
	return a._obj.Type
}
func (a Article)IsLink() bool {
	return false
}
func (a Article)IsObject() bool {
	return true
}

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Fields []Field

type ApiError struct {
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
}

func Errorf(c int, m string, args ...interface{}) *ApiError {
	return &ApiError{c, errors.Errorf(m, args...)}
}

func GetContext() j.Context {
	return j.Context{
		j.Term(j.NilTerm): j.IRI(ap.ActivityBaseURI),
		j.Term("score"): j.IRI("http://littr.me/#ns"),
	}
}

func BuildActorID(a models.Account) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, url.PathEscape(a.Handle)))
}

func BuildCollectionID(a models.Account, o ap.CollectionInterface) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s/%s", AccountsURL, url.PathEscape(a.Handle), getObjectType(o)))
}

func BuildObjectIDFromContent(i models.Content) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s/outbox/%s", AccountsURL, url.PathEscape(i.SubmittedByAccount.Handle), url.PathEscape(i.Hash())))
}
func BuildObjectIDFromVote(v models.Vote) ap.ObjectID {
	att := "liked"
	//if v.Weight < 0 {
	//	att = "disliked"
	//}
	return ap.ObjectID(fmt.Sprintf("%s/%s/%s/%s", AccountsURL, url.PathEscape(v.SubmittedByAccount.Handle), att, url.PathEscape(v.Item.Hash())))
}

func getObjectType (el ap.Item) string {
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
			label = n
			break
		}
	case typeLocalPerson:
		o := val.Interface().(Person)
		for _, n := range o.Name {
			label = n
			break
		}
	}
	return label
}

func BuildObjectURL(b ap.LinkOrURI, el ap.Item) ap.URI {
	pURL := ap.URI(BaseURL)
	if b != nil && b.GetLink() != "" {
		pURL = b.GetLink()
	}

	return ap.URI(fmt.Sprintf("%s/%s", pURL, getObjectType(el)))
}

func HandleApiCall(w http.ResponseWriter, r *http.Request) {
	path := strings.ToLower(chi.URLParam(r, "handle"))
	fmt.Sprintf("%s", strings.Split(path, "/"))
}

func HandleError(w http.ResponseWriter, r *http.Request, code int, errs ...error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
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
		log.Error(err)
		res.Errors = append(res.Errors, e)
	}

	j, _ := json.Marshal(res)
	w.Write(j)
}
