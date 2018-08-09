package app

import (
	log "github.com/sirupsen/logrus"
	"net/http"

	"github.com/mariusor/littr.go/models"

	"fmt"
	"github.com/go-chi/chi"
)

// HandleDomains serves /domains/{domain} request
func HandleDomains(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	ShowItemData = false

	m := userModel{ Title: fmt.Sprintf("Submissions from %s", domain), InvertedTheme: isInverted(r)}

	contentItems, err := models.LoadItemsByDomain(domain, MaxContentItems)
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	m.Items = loadComments(contentItems)

	_, err = LoadVotes(CurrentAccount, m.Items.getItems())
	if err != nil {
		log.Print(err)
	}

	RenderTemplate(r, w, "listing", m)
}
