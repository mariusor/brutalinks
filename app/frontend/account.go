package frontend

import (
	"fmt"
	"github.com/mariusor/littr.go/app"
	"net/http"
	"strconv"

	"github.com/juju/errors"

	"github.com/go-chi/chi"
)

type itemListingModel struct {
	Title         string
	InvertedTheme bool
	NextPage      int
	PrevPage      int
	User          *app.Account
	Items         comments
}

type sessionAccount struct {
	Hash   []byte
	Handle string
}

// ShowAccount serves /~handle request
func ShowAccount(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")

	val := r.Context().Value(app.RepositoryCtxtKey)
	accountLoader, ok := val.(app.CanLoadAccounts)
	if !ok {
		Logger.Errorf("could not load account repository from Context")
		return
	}
	var err error
	a, err := accountLoader.LoadAccount(app.LoadAccountsFilter{Handle: []string{handle}})
	if err != nil {
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	if !a.IsValid() {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("account %s not found", handle))
		return
	}

	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil {
		page = p
		if page <= 0 {
			page = 1
		}
	}
	filter := app.LoadItemsFilter{
		AttributedTo: []app.Hash{a.Hash},
		Page:         page,
		MaxItems:     MaxContentItems,
	}
	if m, err := loadItems(r.Context(), filter); err == nil {
		m.Title = fmt.Sprintf("%s submissions", genitive(a.Handle))
		m.User = &a
		m.InvertedTheme = isInverted(r)
		if len(m.Items) >= MaxContentItems {
			m.NextPage = page + 1
		}
		if page > 1 {
			m.PrevPage = page - 1
		}
		ShowItemData = true

		RenderTemplate(r, w, "listing", m)
	} else {
		HandleError(w, r, http.StatusInternalServerError, err)
	}
}
