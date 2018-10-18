package frontend

import (
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"

	"github.com/mariusor/littr.go/app/models"

	"context"
	"fmt"

	"github.com/go-chi/chi"
	"github.com/juju/errors"
)

func loadItems(c context.Context, filter models.LoadItemsFilter) (itemListingModel, error) {
	m := itemListingModel{}

	itemLoader, ok := models.ContextItemLoader(c)
	if !ok {
		err := errors.Errorf("could not load item repository from Context")
		return m, err
	}
	contentItems, err := itemLoader.LoadItems(filter)
	if err != nil {
		return m, err
	}
	m.Items = loadComments(contentItems)

	acc, ok := models.ContextCurrentAccount(c)
	if acc.IsLogged() {
		votesLoader, ok := models.ContextVoteLoader(c)
		if ok {
			acc.Votes, err = votesLoader.LoadVotes(models.LoadVotesFilter{
				AttributedTo: []models.Hash{acc.Hash},
				ItemKey:      m.Items.getItemsHashes(),
				MaxItems:     MaxContentItems,
			})
			if err != nil {
				Logger.WithFields(log.Fields{}).Error(err)
			}
		} else {
			Logger.WithFields(log.Fields{}).Errorf("could not load vote repository from Context")
		}
	}
	return m, nil
}

// HandleDomains serves /domains/{domain} request
func HandleDomains(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil {
		page = p
		if page <= 0 {
			page = 1
		}
	}
	filter := models.LoadItemsFilter{
		Content:          fmt.Sprintf("http[s]?://%s", domain),
		ContentMatchType: models.MatchFuzzy,
		MediaType:        []string{models.MimeTypeURL},
		Page:             page,
		MaxItems:         MaxContentItems,
	}
	if m, err := loadItems(r.Context(), filter); err == nil {
		m.Title = fmt.Sprintf("Submissions from %s", domain)
		m.InvertedTheme = isInverted(r)

		ShowItemData = false
		if len(m.Items) >= MaxContentItems {
			m.NextPage = page + 1
		}
		if page > 1 {
			m.PrevPage = page - 1
		}
		RenderTemplate(r, w, "listing", m)
	} else {
		HandleError(w, r, http.StatusInternalServerError, err)
	}
}
