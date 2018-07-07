package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"fmt"

	"github.com/mariusor/activitypub.go/jsonld"
)

var Db *sql.DB
var BaseURL string

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

func Errorf(c int, m string, args ...interface{}) *ApiError {
	return &ApiError{c, fmt.Errorf(m, args...)}
}

func GetContext() *jsonld.Context {
	return &jsonld.Context{URL: jsonld.Ref("http://www.w3.org/ns/activitystreams")}
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
