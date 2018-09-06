package app

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
)

type itemListingModel struct {
	Title         string
	InvertedTheme bool
	User          *models.Account
	Items         comments
}

// ShowAccount serves /~handle request
func ShowAccount(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")

	val := r.Context().Value(RepositoryCtxtKey)
	accountLoader, ok := val.(models.CanLoadAccounts)
	if ok {
		log.Infof("loaded repository of type %T", accountLoader)
	} else {
		log.Errorf("could not load item loader service from Context")
		return
	}
	var err error
	a, err := accountLoader.LoadAccount(models.LoadAccountFilter{Handle: handle})
	if err != nil {
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	if !a.IsValid() {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("account %s not found", handle))
		return
	}
	filter := models.LoadItemsFilter{
		AttributedTo: []string{a.Hash},
		MaxItems:     MaxContentItems,
	}
	if m, err := loadItems(r.Context(), filter); err == nil {
		m.Title = fmt.Sprintf("%s submissions", genitive(a.Handle))
		m.User = &a
		m.InvertedTheme = isInverted(r)

		ShowItemData = true

		RenderTemplate(r, w, "listing", m)
	} else {
		HandleError(w, r, http.StatusInternalServerError, err)
	}
}
