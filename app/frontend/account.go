package frontend

import (
	"fmt"
	"github.com/go-ap/errors"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/qstring"
	"net/http"
	"net/url"

	"github.com/go-chi/chi"
)

// Paginator is an interface for paginating collections
type Paginator interface {
	NextPage() int
	PrevPage() int
}

type itemListingModel struct {
	Title          string
	User           *app.Account
	Items          []HasType
	HideText       bool
	nextPage       int
	prevPage       int
}

func (i itemListingModel) NextPage() int {
	return i.nextPage
}

func (i itemListingModel) PrevPage() int {
	return i.prevPage
}

// ShowAccount serves /~handler request
func (h *handler) ShowAccount(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")

	accountLoader, ok := app.ContextAccountLoader(r.Context())
	if !ok {
		err := errors.Errorf("could not load account repository from Context")
		h.logger.Error(err.Error())
		h.HandleErrors(w, r, err)
		return
	}
	var err error
	accounts, cnt, err := accountLoader.LoadAccounts(app.Filters{LoadAccountsFilter: app.LoadAccountsFilter{Handle: []string{handle}}})
	if err != nil {
		h.HandleErrors(w, r, err)
		return
	}
	if cnt == 0 {
		h.HandleErrors(w, r, errors.NotFoundf("account %q not found", handle))
		return
	}
	if cnt > 1 {
		h.HandleErrors(w, r, errors.NotFoundf("too many %q accounts found", handle))
		return
	}

	filter := app.Filters{
		LoadItemsFilter: app.LoadItemsFilter{},
		MaxItems:        MaxContentItems,
		Page:            1,
	}
	for _, a := range accounts {
		filter.LoadItemsFilter.AttributedTo = append(filter.LoadItemsFilter.AttributedTo, a.Hash)
	}

	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}
	baseURL, _ := url.Parse(h.conf.BaseURL)
	m := itemListingModel{}

	m.Title = fmt.Sprintf("%s: %s submissions", baseURL.Host, genitive(handle))
	m.User, _ = accounts.First()
	comments, err := loadItems(r.Context(), filter, h.account(r), h.logger)
	if err != nil {
		h.HandleErrors(w, r, errors.NewNotValid(err, "unable to load items"))
	}
	for _, com := range comments {
		m.Items = append(m.Items, com)
	}
	if len(comments) >= filter.MaxItems {
		m.nextPage = filter.Page + 1
	}
	if filter.Page > 1 {
		m.prevPage = filter.Page - 1
	}
	h.RenderTemplate(r, w, "user", m)
}

func (h *handler) FollowAccount(w http.ResponseWriter, r *http.Request) {
	loggedAccount := h.account(r)
	if !loggedAccount.IsValid() {
		err := errors.Unauthorizedf("invalid logged account")
		h.logger.Error(err.Error())
		h.HandleErrors(w, r, err)
		return
	}

	handle := chi.URLParam(r, "handle")
	accountLoader, ok := app.ContextAccountLoader(r.Context())
	if !ok {
		err := errors.Errorf("could not load account repository from Context")
		h.logger.Error(err.Error())
		h.HandleErrors(w, r, err)
		return
	}
	var err error
	accounts, cnt, err := accountLoader.LoadAccounts(app.Filters{LoadAccountsFilter: app.LoadAccountsFilter{Handle: []string{handle}}})
	if err != nil {
		h.HandleErrors(w, r, err)
		return
	}
	if cnt == 0 {
		h.HandleErrors(w, r, errors.NotFoundf("account %q not found", handle))
		return
	}
	if cnt > 1 {
		h.HandleErrors(w, r, errors.NotFoundf("too many %q accounts found", handle))
		return
	}
	toFollow, _ := accounts.First()
	err = h.storage.FollowAccount(*loggedAccount, *toFollow)
	if err != nil {
		h.HandleErrors(w, r, err)
		return
	}
	h.Redirect(w, r, AccountPermaLink(*toFollow), http.StatusSeeOther)
}
