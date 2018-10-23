package api

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/go-chi/chi"

	ap "github.com/mariusor/littr.go/app/activitypub"
	"github.com/mariusor/littr.go/app/models"

	log "github.com/sirupsen/logrus"

	j "github.com/mariusor/activitypub.go/jsonld"
)

// POST /api - not implemented yet - but we should have all information in the CreateActivity body
// PUT /api/actors/{handle}/{collection}/{item_hash}
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
		act := ap.Activity{}
		if err := j.Unmarshal(body, &act); err != nil {
			Logger.WithFields(log.Fields{}).Errorf("json-ld unmarshal error: %s", err)
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		it := models.Item{}
		if err := it.FromActivityPubItem(act); err != nil {
			Logger.WithFields(log.Fields{}).Errorf("json-ld unmarshal error: %s", err)
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		if repo, ok := models.ContextItemSaver(r.Context()); ok {
			newIt, err := repo.SaveItem(it)
			if err != nil {
				Logger.WithFields(log.Fields{
					"item":    it.Hash,
					"account": it.SubmittedBy.Hash,
				}).Error(err)
				HandleError(w, r, http.StatusInternalServerError, err)
				return
			}
			if newIt.UpdatedAt.IsZero() {
				// we need to make a difference between created vote and updated vote
				// created - http.StatusCreated
				status = http.StatusCreated
				location = fmt.Sprintf("/api/actors/%s/%s/%s", newIt.SubmittedBy.Handle, col, newIt.Hash)
			} else {
				// updated - http.StatusOK
				status = http.StatusOK
			}
		}
	case "liked":
		act := ap.Activity{}
		if err := j.Unmarshal(body, &act); err != nil {
			Logger.WithFields(log.Fields{}).Errorf("json-ld unmarshal error: %s", err)
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		v := models.Vote{}
		if err := v.FromActivityPubItem(act); err != nil {
			Logger.WithFields(log.Fields{}).Errorf("json-ld unmarshal error: %s", err)
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
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
				location = fmt.Sprintf("/api/actors/%s/%s/%s", newVot.SubmittedBy.Handle, col, newVot.Item.Hash)
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
