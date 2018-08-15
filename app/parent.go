package app

import (
	"net/http"

	"github.com/mariusor/littr.go/models"

	"github.com/go-chi/chi"
)

// HandleParent serves /parent/{hash}/{parent} request
func HandleParent(w http.ResponseWriter, r *http.Request) {
	p, err :=  models.LoadItemParent(chi.URLParam(r, "hash"))
	if err != nil {
		HandleError(w, r, StatusUnknown, err)
		return
	}

	url := permaLink(p)
	Redirect(w, r, url, http.StatusMovedPermanently)
}

// HandleOp serves /op/{hash}/{parent} request
func HandleOp(w http.ResponseWriter, r *http.Request) {
	p, err :=  models.LoadItemOP(chi.URLParam(r, "hash"))
	if err != nil {
		HandleError(w, r, StatusUnknown, err)
		return
	}

	url := permaLink(p)
	Redirect(w, r, url, http.StatusMovedPermanently)
}
