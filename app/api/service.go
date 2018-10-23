package api

import (
	"github.com/juju/errors"
	"net/http"
)

// GET /api/service
func HandleService(w http.ResponseWriter, r *http.Request) {
	HandleError(w, r, http.StatusNotImplemented, errors.New("Not implemented yet"))
}
