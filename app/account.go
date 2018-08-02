package app

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"

	"github.com/mariusor/littr.go/models"
	"github.com/juju/errors"
	"github.com/go-chi/chi"
	"github.com/mariusor/activitypub.go/activitypub"
	"github.com/mariusor/activitypub.go/jsonld"
	"io/ioutil"
)

type userModel struct {
	Title         string
	InvertedTheme bool
	User          *Account
	Items         []Item
}

func loadFromAPPerson(p activitypub.Person) (Account, error) {
	u := Account{
		UpdatedAt: p.Updated,
		Handle:    p.Name.First(),
		CreatedAt: p.Published,
	}
	return u, nil
}

// HandleUser serves /~handle request
func HandleUser(w http.ResponseWriter, r *http.Request) {
	m := userModel{InvertedTheme: IsInverted(r)}
	log.WithFields(log.Fields{
		"account": "account page",
	})
	found := false

	handle := chi.URLParam(r, "handle")
	if true {
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
			m, _ :=  loadFromAPPerson(p)
			log.Debugf("resp %#v", m)
		}
	}
	u := models.Account{}
	selAcct := `select "id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags" from "accounts" where "handle" = $1`
	{
		rows, err := Db.Query(selAcct, handle)
		if err != nil {
			HandleError(w, r, StatusUnknown, err)
			return
		}
		for rows.Next() {
			err = rows.Scan(&u.Id, &u.Key, &u.Handle, &u.Email, &u.Score, &u.CreatedAt, &u.UpdatedAt, &u.Metadata, &u.Flags)
			if err != nil {
				HandleError(w, r, StatusUnknown, err)
				return
			}
			found = true
		}
		m.Title = fmt.Sprintf("Submissions by %s", u.Handle)
		m.User = &Account{
			Id:        u.Id,
			Hash:      u.Hash(),
			flags:     u.Flags,
			UpdatedAt: u.UpdatedAt,
			Handle:    u.Handle,
			metadata:  u.Metadata,
			Score:     u.Score,
			CreatedAt: u.CreatedAt,
			Email:     u.Email,
		}
	}

	if !found {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("user %q not found", handle))
		return
	}

	selC := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "content_items"."flags", "content_items"."metadata", "accounts"."handle" f from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where "submitted_by" = $1 order by "submitted_at" desc`
	{
		rows, err := Db.Query(selC, u.Id)
		if err != nil {
			HandleError(w, r, StatusUnknown, err)
			return
		}
		for rows.Next() {
			p := models.Content{}
			var handle string
			err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.Flags, &p.Metadata, &handle)
			if err != nil {
				HandleError(w, r, StatusUnknown, err)
				return
			}
			l := LoadItem(p, handle)
			m.Items = append(m.Items, l)
		}
	}
	_, err := LoadVotes(CurrentAccount, m.Items)
	if err != nil {
		log.Error(err)
	}
	err = SessionStore.Save(r, w, GetSession(r))
	if err != nil {
		log.Error(err)
	}

	RenderTemplate(r, w, "user", m)
}
