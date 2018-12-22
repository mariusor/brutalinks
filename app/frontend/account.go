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
	Title    string
	NextPage int
	PrevPage int
	User     *app.Account
	Items    comments
	HideText bool
}

type sessionAccount struct {
	Hash   []byte
	Handle string
}

// ShowAccount serves /~handler request
func (h *handler) ShowAccount(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")

	val := r.Context().Value(app.RepositoryCtxtKey)
	accountLoader, ok := val.(app.CanLoadAccounts)
	if !ok {
		h.logger.Error("could not load account repository from Context")
		return
	}
	var err error
	a, err := accountLoader.LoadAccount(app.LoadAccountsFilter{Handle: []string{handle}})
	if err != nil {
		h.HandleError(w, r, err)
		return
	}
	if !a.IsValid() {
		h.HandleError(w, r, errors.NotFoundf("account %q not found", handle))
		return
	}

	filter := app.LoadItemsFilter{
		AttributedTo: app.Hashes{a.Hash},
		MaxItems:     MaxContentItems,
		Page:         1,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}
	if m, err := loadItems(r.Context(), filter, &h.account, h.logger); err == nil {
		m.Title = fmt.Sprintf("%s submissions", genitive(a.Handle))
		m.User = &a
		if len(m.Items) >= MaxContentItems {
			m.NextPage = filter.Page + 1
		}
		if filter.Page > 1 {
			m.PrevPage = filter.Page - 1
		}

		h.RenderTemplate(r, w, "user", m)
	} else {
		h.HandleError(w, r, errors.NewNotValid(err, "unable to load items"))
	}
}
