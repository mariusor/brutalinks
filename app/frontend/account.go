package frontend

import (
	"fmt"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/qstring"
	"net/http"

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
		Logger.Error("could not load account repository from Context")
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

	filter := app.LoadItemsFilter{
		AttributedTo: []app.Hash{a.Hash},
		MaxItems:     MaxContentItems,
		Page:         1,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		Logger.Debug("unable to load url parameters")
	}
	if m, err := loadItems(r.Context(), filter); err == nil {
		m.Title = fmt.Sprintf("%s submissions", genitive(a.Handle))
		m.User = &a
		m.InvertedTheme = isInverted(r)
		if len(m.Items) >= MaxContentItems {
			m.NextPage = filter.Page + 1
		}
		if filter.Page > 1 {
			m.PrevPage = filter.Page - 1
		}
		ShowItemData = true

		RenderTemplate(r, w, "user", m)
	} else {
		HandleError(w, r, http.StatusInternalServerError, err)
	}
}
