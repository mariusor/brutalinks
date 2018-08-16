package app

import (
	log "github.com/sirupsen/logrus"
	"net/http"

	"github.com/mariusor/littr.go/models"

	"fmt"
	"github.com/go-chi/chi"
	"github.com/juju/errors"
	"context"
)

func loadItems(c context.Context, filter models.LoadItemsFilter) (itemListingModel, error) {
	m := itemListingModel{}
	val := c.Value(ServiceCtxtKey)
	itemLoader, ok := val.(models.CanLoadItems)
	if ok {
		log.Infof("loaded LoaderService of type %T", itemLoader)
	} else {
		err := errors.Errorf("could not load item loader service from Context")
		return m, err
	}
	contentItems, err := itemLoader.LoadItems(filter)
	if err != nil {
		return m, err
	}
	m.Items = loadComments(contentItems)

	if CurrentAccount.IsLogged() {
		votesLoader, ok := val.(models.CanLoadVotes)
		if ok {
			log.Infof("loaded LoaderService of type %T", itemLoader)
			CurrentAccount.Votes, err = votesLoader.LoadVotes(models.LoadVotesFilter{
				SubmittedBy: []string{CurrentAccount.Hash,},
				ItemKey:     m.Items.getItemsHashes(),
				MaxItems:    MaxContentItems,
			})
			if err != nil {
				log.Error(err)
			}
		} else {
			log.Errorf("could not load vote loader service from Context")
		}
	}
	return m, nil
}

// HandleDomains serves /domains/{domain} request
func HandleDomains(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")

	filter := models.LoadItemsFilter{
		Content: fmt.Sprintf("http[s]?://%s", domain),
		ContentMatchType: models.MatchFuzzy,
		MediaType: []string{models.MimeTypeURL},
		MaxItems: MaxContentItems,
	}
	if m, err := loadItems(r.Context(), filter); err == nil {
		m.Title = fmt.Sprintf("Submissions from %s", domain)
		m.InvertedTheme = isInverted(r)

		ShowItemData = false

		RenderTemplate(r, w, "listing", m)
	} else {
		HandleError(w, r, http.StatusInternalServerError, err)
	}
}
