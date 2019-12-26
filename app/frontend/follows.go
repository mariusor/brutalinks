package frontend

import (
	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/mariusor/littr.go/app"
	"net/http"
)

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

func (h *handler) HandleFollowRequest(w http.ResponseWriter, r *http.Request) {
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
	accounts, cnt, err := accountLoader.LoadAccounts(app.Filters{LoadAccountsFilter: app.LoadAccountsFilter{Handle: []string{handle}}})
	if err != nil {
		h.HandleErrors(w, r, err)
		return
	}
	if cnt == 0 {
		h.HandleErrors(w, r, errors.NotFoundf("account %q not found", handle))
		return
	}
	follower, _ := accounts.First()
	
	accept := false
	action := chi.URLParam(r, "action")
	if action == "accept" {
		accept = true
	}
	loader, ok := app.ContextLoader(r.Context())
	if !ok {
		err := errors.Errorf("could not load account repository from Context")
		h.logger.Error(err.Error())
		h.HandleErrors(w, r, err)
		return
	}

	followRequests, cnt, err := loader.LoadFollowRequests(loggedAccount, app.Filters{
		LoadFollowRequestsFilter: app.LoadFollowRequestsFilter{
			Actor: app.Hashes{app.Hash(follower.Metadata.ID)},
			On: app.Hashes{app.Hash(loggedAccount.Metadata.ID)},
		},
	})
	if err != nil {
		h.HandleErrors(w, r, err)
		return
	}
	if cnt == 0 {
		h.HandleErrors(w, r, errors.NotFoundf("follow request not found"))
		return
	}
	follow := followRequests[0]
	err = h.storage.SendFollowResponse(follow, accept)
	if err != nil {
		h.HandleErrors(w, r, err)
		return
	}
	backUrl := r.Header.Get("Referer")
	h.Redirect(w, r, backUrl, http.StatusSeeOther)
}
