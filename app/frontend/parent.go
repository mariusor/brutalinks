package frontend

import (
	"github.com/mariusor/littr.go/internal/errors"
	"github.com/mariusor/littr.go/app"
	"net/http"

	"github.com/go-chi/chi"
)

// HandleItemRedirect serves /i/{hash} request
func (h *handler) HandleItemRedirect(w http.ResponseWriter, r *http.Request) {
	val := r.Context().Value(app.RepositoryCtxtKey)
	itemLoader, ok := val.(app.CanLoadItems)
	if !ok {
		h.logger.Error("could not load item repository from Context")
		return
	}
	p, err := itemLoader.LoadItem(app.LoadItemsFilter{
		Key:      app.Hashes{app.Hash(chi.URLParam(r, "hash"))},
		MaxItems: 1,
	})
	if err != nil {
		h.HandleErrors(w, r, errors.NewNotValid(err, "oops!"))
		return
	}
	url := ItemPermaLink(p)
	h.Redirect(w, r, url, http.StatusMovedPermanently)
}
