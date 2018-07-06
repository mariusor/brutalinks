package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

var Db *sql.DB

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Fields []Field

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
