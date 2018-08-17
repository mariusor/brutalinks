package app

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/mariusor/littr.go/models"
)

type itemListingModel struct {
	Title         string
	InvertedTheme bool
	User          *models.Account
	Items         comments
}

// HandleUser serves /~handle request
func HandleUser(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")
	a, _ := models.Service.LoadAccount(models.LoadAccountFilter{Handle: handle})

	filter := models.LoadItemsFilter{
		SubmittedBy: []string{a.Hash},
		MaxItems:    MaxContentItems,
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
