package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"fmt"
		"strings"
	"reflect"
	"os"

	ap "github.com/mariusor/activitypub.go/activitypub"
	j "github.com/mariusor/activitypub.go/jsonld"
	"github.com/go-chi/chi"
)

var Db *sql.DB
var BaseURL string
var AccountsURL string

const NotFound = 404
const InternalError = 500

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
	return &ApiError{c, fmt.Errorf(m, args...)}
}

func GetContext() j.Ref {
	return j.Ref(ap.ActivityBaseURI)
}

func BuildObjectID(path string, parent ap.Item, cur ap.Item) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s%s/%s", path, parent.GetID(), cur.GetID()))
}

func BuildObjectURL(b ap.LinkOrURI, el ap.Item) ap.URI {
	var (
		label            = ""
		typeOutbox       = reflect.TypeOf(ap.Outbox{})
		typeOutboxStream = reflect.TypeOf(ap.OutboxStream{})
		typeInbox        = reflect.TypeOf(ap.Inbox{})
		typeInboxStream  = reflect.TypeOf(ap.InboxStream{})
		typeLiked        = reflect.TypeOf(ap.Liked{})
		typeLikedCollection        = reflect.TypeOf(ap.LikedCollection{})
		typePerson        = reflect.TypeOf(ap.Person{})
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
	}
	pURL := b.GetLink()
	if pURL == "" {
		pURL =  ap.URI(BaseURL)
	}

	return ap.URI(fmt.Sprintf("%s/%s", b.GetLink(), label))
}

func HandleApiCall(w http.ResponseWriter, r *http.Request) {
	path := strings.ToLower(chi.URLParam(r,"handle"))
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
		res.Errors = append(res.Errors, e)
	}

	j, _ := json.Marshal(res)
	w.Write(j)
}
