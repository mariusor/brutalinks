package frontend

import (
	"net/http"

	"github.com/mariusor/littr.go/app/models"

	"github.com/go-chi/chi"
	log "github.com/sirupsen/logrus"
)

// HandleItemRedirect serves /item/{hash} request
func HandleItemRedirect(w http.ResponseWriter, r *http.Request) {
	val := r.Context().Value(models.RepositoryCtxtKey)
	itemLoader, ok := val.(models.CanLoadItems)
	if !ok {
		Logger.WithFields(log.Fields{}).Errorf("could not load item repository from Context")
		return
	}
	p, err := itemLoader.LoadItem(models.LoadItemsFilter{
		Key:      []string{chi.URLParam(r, "hash")},
		MaxItems: 1,
	})
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	url := ItemPermaLink(p)
	Redirect(w, r, url, http.StatusMovedPermanently)
}
