package app

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"

	"github.com/go-chi/chi"
		"github.com/mariusor/activitypub.go/activitypub"
	"github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/models"
	"io/ioutil"
)

type userModel struct {
	Title         string
	InvertedTheme bool
	User          *models.Account
	Items         comments
}

func loadFromAPPerson(p activitypub.Person) (models.Account, error) {
	u := models.Account{
		UpdatedAt: p.Updated,
		Handle:    p.Name.First(),
		CreatedAt: p.Published,
	}
	return u, nil
}

// HandleUser serves /~handle request
func HandleUser(w http.ResponseWriter, r *http.Request) {
	m := userModel{InvertedTheme: isInverted(r)}
	log.WithFields(log.Fields{
		"account": "account page",
	})
	ShowItemData = true

	handle := chi.URLParam(r, "handle")
	if false {
		resp, err := http.Get(fmt.Sprintf("http://localhost:3000/api/accounts/%s", handle))
		if err != nil {
			log.Error(err)
		}
		if resp != nil {
			defer resp.Body.Close()
			body, terr := ioutil.ReadAll(resp.Body)
			if terr != nil {
				log.Error(terr)
			}
			p := activitypub.Person{}
			uerr := jsonld.Unmarshal(body, &p)
			if uerr != nil {
				log.Error(uerr)
			}
			log.Infof("resp %s", body)
			log.Infof("resp %#v", p)
			m, _ := loadFromAPPerson(p)
			log.Debugf("resp %#v", m)
		}
	}

	a, _ := models.Service.LoadAccount(models.LoadAccountFilter{Handle:handle})
	m.Title = fmt.Sprintf("%s submissions", genitive(a.Handle))
	m.User = &a

	items, err := models.LoadItemsSubmittedBy(handle)
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	m.Items = loadComments(items)

	if CurrentAccount.IsLogged() {
		CurrentAccount.Votes, err = models.Service.LoadVotes(models.LoadVotesFilter{
			SubmittedBy: []string{CurrentAccount.Hash,},
			ItemKey:     m.Items.getItemsHashes(),
			MaxItems:    MaxContentItems,
		})

		if err != nil {
			log.Error(err)
		}
	}

	RenderTemplate(r, w, "listing", m)
}
