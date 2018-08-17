package app

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/mariusor/littr.go/models"

	"github.com/go-chi/chi"
	log "github.com/sirupsen/logrus"
)

// HandleParent serves /parent/{hash} request
// HandleParent serves /op/{hash} request
func HandleParent(w http.ResponseWriter, r *http.Request) {
	val := r.Context().Value(ServiceCtxtKey)
	itemLoader, ok := val.(models.CanLoadItems)
	if ok {
		log.Infof("loaded LoaderService of type %T", itemLoader)
	} else {
		log.Errorf("could not load item loader service from Context")
		return
	}
	p, err := itemLoader.LoadItem(models.LoadItemsFilter{
		Key:      []string{chi.URLParam(r, "hash")},
		MaxItems: 1,
	})
	if err != nil {
		HandleError(w, r, StatusUnknown, err)
		return
	}

	parent := chi.URLParam(r, "parent")
	var which *models.Item
	which = p.Parent
	if parent == "op" {
		which = p.OP
	}

	if which == nil {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("not found"))
	}
	if parent, err := itemLoader.LoadItem(models.LoadItemsFilter{
		Key:      []string{which.Hash},
		MaxItems: 1,
	}); err != nil {
		HandleError(w, r, StatusUnknown, err)
	} else {
		url := permaLink(parent)
		Redirect(w, r, url, http.StatusMovedPermanently)
	}
}
