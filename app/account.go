package app

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/juju/errors"
	"github.com/mariusor/activitypub.go/activitypub"
	"github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/models"
	"io/ioutil"
)

type userModel struct {
	Title         string
	InvertedTheme bool
	User          *Account
	Items         comments
}

func loadFromAPPerson(p activitypub.Person) (Account, error) {
	u := Account{
		UpdatedAt: p.Updated,
		Handle:    p.Name.First(),
		CreatedAt: p.Published,
	}
	return u, nil
}

func loadFromModel(a models.Account) (Account, error) {
	return Account{
		Id:        a.Id,
		Hash:      a.Hash(),
		flags:     a.Flags,
		UpdatedAt: a.UpdatedAt,
		Handle:    a.Handle,
		metadata:  a.Metadata,
		Score:     a.Score,
		CreatedAt: a.CreatedAt,
		Email:     a.Email,
	}, nil
}

// HandleUser serves /~handle request
func HandleUser(w http.ResponseWriter, r *http.Request) {
	m := userModel{InvertedTheme: IsInverted(r)}
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
	u, err := models.LoadAccount(Db, handle)
	if err != nil {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("user %q not found", handle))
		return
	}
	m.Title = fmt.Sprintf("%s submissions", genitive(u.Handle))
	a, _ := loadFromModel(u)
	m.User = &a

	items, err := models.LoadItemsSubmittedBy(Db, handle)
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	m.Items = loadComments(items)

	_, err = LoadVotes(CurrentAccount, m.Items.getItems())
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	if err != nil {
		log.Error(err)
	}
	err = SessionStore.Save(r, w, GetSession(r))
	if err != nil {
		log.Error(err)
	}

	RenderTemplate(r, w, "listing", m)
}
