package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/mariusor/activitypub.go/jsonld"
)

var Db *sql.DB
var BaseURL string

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Fields []Field

func GetContext() *jsonld.Context {
	return &jsonld.Context{URL: jsonld.Ref(BaseURL)}
}

func HandleError(w http.ResponseWriter, r *http.Request, err error, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)

	e := struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{
		code,
		err.Error(),
	}

	j, _ := json.Marshal(e)
	w.Write(j)
}
