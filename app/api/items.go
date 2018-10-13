package api

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path"

	"github.com/go-chi/chi"

	"github.com/mariusor/littr.go/app/models"

	log "github.com/sirupsen/logrus"

	ap "github.com/mariusor/activitypub.go/activitypub"
	as "github.com/mariusor/activitypub.go/activitystreams"
	j "github.com/mariusor/activitypub.go/jsonld"
)

func loadVote(body []byte) models.Vote {
	act := Activity{}

	if err := j.Unmarshal(body, &act); err != nil {
		Logger.WithFields(log.Fields{}).Errorf("json-ld unmarshal error: %s", err)
	}

	actor := act.Actor
	ob := act.Object

	var accountHash models.Hash
	if actor.IsLink() {
		// just the ObjectID
		accountHash = models.Hash(path.Base(string(actor.(as.IRI))))
	}
	if actor.IsObject() {
		// full Actor struct
		accountHash = models.Hash(path.Base(string(*actor.GetID())))
	}
	var itemHash string
	if ob.IsLink() {
		// just the ObjectID
		itemHash = path.Base(string(ob.(as.IRI)))
	}
	if ob.IsObject() {
		// full Object struct
		itemHash = path.Base(string(*ob.GetID()))
	}
	v := models.Vote{
		SubmittedBy: &models.Account{
			Hash: accountHash,
		},
		Item: &models.Item{
			Hash: models.Hash(itemHash),
		},
	}
	if act.GetType() == as.LikeType {
		v.Weight = 1
	}
	if act.GetType() == as.DislikeType {
		v.Weight = -1
	}

	return v
}

func loadItem(body []byte) models.Item {
	act := ap.CreateActivity{}

	if err := j.Unmarshal(body, &act); err != nil {
		Logger.WithFields(log.Fields{}).Errorf("json-ld unmarshal error: %s", err)
	}

	actor := act.Activity.Actor
	ob := act.Activity.Object
	it := models.Item{
		SubmittedBy: &models.Account{
			Hash: getHashFromAP(actor),
		},
	}
	if ob.IsObject() {
		// full Object struct
		obj, ok := act.Activity.Object.(*as.Object)
		if !ok {
			Logger.Errorf("invalid object in %T activity", act)
		} else {
			if len(obj.ID) > 0 {
				it.Hash = models.Hash(path.Base(string(obj.ID)))
			}
			title := jsonUnescape(as.NaturalLanguageValue(obj.Name).First())
			content := jsonUnescape(as.NaturalLanguageValue(obj.Content).First())

			it.Data = content
			it.Title = title
			it.MimeType = string(obj.MediaType)
			if obj.Context != nil {
				it.OP = &models.Item{
					Hash: getHashFromAP(obj.Context),
				}
			}
			if obj.InReplyTo != nil {
				it.Parent = &models.Item{
					Hash: getHashFromAP(obj.InReplyTo),
				}
			}
		}
	}
	it.SubmittedBy = &models.Account{
		Hash: getHashFromAP(actor),
	}
	it.Hash = getHashFromAP(ob)

	return it
}

// POST /api - not implemented yet - but we should have all information in the CreateActivity body
// PUT /api/accounts/{handle}/{collection}/{item_hash}
func UpdateItem(w http.ResponseWriter, r *http.Request) {
	// verify signature header:
	// Signature: keyId="https://my-example.com/actor#main-key",headers="(request-target) host date",signature="..."

	var body []byte
	var err error
	defer r.Body.Close()
	status := http.StatusInternalServerError

	if body, err = ioutil.ReadAll(r.Body); err != nil {
		Logger.WithFields(log.Fields{}).Errorf("request body read error: %s", err)
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}

	location := ""
	col := chi.URLParam(r, "collection")
	switch col {
	case "outbox":
		it := loadItem(body)
		if repo, ok := models.ContextItemSaver(r.Context()); ok {
			newIt, err := repo.SaveItem(it)
			if err != nil {
				Logger.WithFields(log.Fields{"saveItem": it.SubmittedBy.Hash}).Error(err)
				HandleError(w, r, http.StatusInternalServerError, err)
				return
			}
			if newIt.UpdatedAt.IsZero() {
				// we need to make a difference between created vote and updated vote
				// created - http.StatusCreated
				status = http.StatusCreated
				location = fmt.Sprintf("/api/accounts/%s/%s/%s", newIt.SubmittedBy.Handle, col, newIt.Hash)
			} else {
				// updated - http.StatusOK
				status = http.StatusOK
			}
		}
	case "liked":
		v := loadVote(body)
		if repo, ok := models.ContextVoteSaver(r.Context()); ok {
			newVot, err := repo.SaveVote(v)
			if err != nil {
				Logger.WithFields(log.Fields{"saveVote": v.SubmittedBy.Hash}).Error(err)
				HandleError(w, r, http.StatusInternalServerError, err)
				return
			}
			if newVot.UpdatedAt.IsZero() {
				// we need to make a difference between created vote and updated vote
				// created - http.StatusCreated
				status = http.StatusCreated
				location = fmt.Sprintf("/api/accounts/%s/%s/%s", newVot.SubmittedBy.Handle, col, newVot.Item.Hash)
			} else {
				// updated - http.StatusOK
				status = http.StatusOK
			}
		}
	}

	w.Header().Add("Content-Type", "application/activity+json; charset=utf-8")
	if status == http.StatusCreated {
		w.Header().Add("Location", location)
	}
	w.WriteHeader(status)
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	if status >= 400 {
		w.Write([]byte(`{"status": "nok"}`))
	} else {
		w.Write([]byte(`{"status": "ok"}`))
	}
}
