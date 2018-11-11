package frontend

import (
	"context"
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/qstring"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/juju/errors"
)

func loadItems(c context.Context, filter app.LoadItemsFilter) (itemListingModel, error) {
	m := itemListingModel{}

	itemLoader, ok := app.ContextItemLoader(c)
	if !ok {
		err := errors.Errorf("could not load item repository from Context")
		return m, err
	}
	contentItems, err := itemLoader.LoadItems(filter)
	if err != nil {
		return m, err
	}
	m.Items = loadComments(contentItems)

	acc, ok := app.ContextCurrentAccount(c)
	if acc.IsLogged() {
		votesLoader, ok := app.ContextVoteLoader(c)
		if ok {
			acc.Votes, err = votesLoader.LoadVotes(app.LoadVotesFilter{
				AttributedTo: []app.Hash{acc.Hash},
				ItemKey:      m.Items.getItemsHashes(),
				MaxItems:     MaxContentItems,
			})
			if err != nil {
				Logger.Error(err.Error())
			}
		} else {
			Logger.Error("could not load vote repository from Context")
		}
	}
	return m, nil
}

// HandleDomains serves /domains/{domain} request
func HandleDomains(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	filter := app.LoadItemsFilter{
		Content:          fmt.Sprintf("http[s]?://%s", domain),
		ContentMatchType: app.MatchFuzzy,
		MediaType:        []string{app.MimeTypeURL},
		MaxItems:         MaxContentItems,
		Page:             1,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		Logger.Debug("unable to load url parameters")
	}
	if m, err := loadItems(r.Context(), filter); err == nil {
		m.Title = fmt.Sprintf("Submissions from %s", domain)
		m.InvertedTheme = isInverted(r)

		ShowItemData = false
		if len(m.Items) >= MaxContentItems {
			m.NextPage = filter.Page + 1
		}
		if filter.Page > 1 {
			m.PrevPage = filter.Page - 1
		}
		RenderTemplate(r, w, "listing", m)
	} else {
		HandleError(w, r, http.StatusInternalServerError, err)
	}
}
