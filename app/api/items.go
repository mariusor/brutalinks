package api

import (
	"io/ioutil"
	"net/http"
	"path"

	"github.com/mariusor/littr.go/app/models"

	log "github.com/sirupsen/logrus"

	ap "github.com/mariusor/activitypub.go/activitypub"
	j "github.com/mariusor/activitypub.go/jsonld"
)

// POST /api - not implemented yet - but we should have all information in the CreateActivity body
// PUT /api/accounts/{handle}/{collection}/{item_hash}
func UpdateItem(w http.ResponseWriter, r *http.Request) {
	// verify signature header:
	// Signature: keyId="https://my-example.com/actor#main-key",headers="(request-target) host date",signature="..."

	act := ap.LikeActivity{}
	var body []byte
	var err error
	if r != nil {
		defer r.Body.Close()

		if body, err = ioutil.ReadAll(r.Body); err != nil {
			log.WithFields(log.Fields{}).Errorf("request body read error: %s", err)
		}
		if err := j.Unmarshal(body, &act); err != nil {
			log.WithFields(log.Fields{}).Errorf("json-ld unmarshal error: %s", err)
		}
	}

	actor := act.Activity.Actor
	ob := act.Activity.Object

	var accountHash string
	if actor.IsLink() {
		// just the ObjectID
		accountHash = path.Base(string(actor.(ap.IRI)))
	}
	if actor.IsObject() {
		// full Actor struct
		accountHash = path.Base(string(*actor.GetID()))
	}
	var itemHash string
	if ob.IsLink() {
		// just the ObjectID
		itemHash = path.Base(string(ob.(ap.IRI)))
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
			Hash: itemHash,
		},
	}
	if act.Activity.GetType() == ap.LikeType {
		v.Weight = 1
	}
	if act.Activity.GetType() == ap.DislikeType {
		v.Weight = -1
	}

	ctxt := r.Context().Value(RepositoryCtxtKey)
	if repository, ok := ctxt.(models.CanSaveVotes); ok {
		newVot, err := repository.SaveVote(v)
		if err != nil {
			log.WithFields(log.Fields{}).Error(err)
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		if newVot.SubmittedAt != newVot.UpdatedAt {
			// we need to make a difference between created vote and updated vote
			// created - http.StatusCreated
			// updated - http.StatusOK
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte(`{"status": "ok"}`))
}
