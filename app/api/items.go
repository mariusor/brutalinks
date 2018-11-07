package api

import (
	"fmt"
	"github.com/mariusor/littr.go/app"
	"io/ioutil"
	"net/http"

	"github.com/go-chi/chi"

	ap "github.com/mariusor/littr.go/app/activitypub"

	log "github.com/inconshreveable/log15"

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
		Logger.Error("request body read error", log.Ctx{"err": err})
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}

	location := ""
	col := chi.URLParam(r, "collection")
	switch col {
	case "outbox":
		act := ap.Activity{}
		if err := j.Unmarshal(body, &act); err != nil {
			Logger.Error("json-ld unmarshal error", log.Ctx{"err": err})
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		it := app.Item{}
		if err := it.FromActivityPubItem(act); err != nil {
			Logger.Error("json-ld unmarshal error", log.Ctx{"err": err})
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		if repo, ok := app.ContextItemSaver(r.Context()); ok {
			newIt, err := repo.SaveItem(it)
			if err != nil {
				Logger.Error(err.Error(), log.Ctx{
					"item":    it.Hash,
					"account": it.SubmittedBy.Hash,
				})
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
			Logger.Error("json-ld unmarshal error", log.Ctx{"err": err})
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		v := app.Vote{}
		if err := v.FromActivityPubItem(act); err != nil {
			Logger.Error("json-ld unmarshal error", log.Ctx{"err": err})
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		if repo, ok := app.ContextVoteSaver(r.Context()); ok {
			newVot, err := repo.SaveVote(v)
			if err != nil {
				Logger.Error(err.Error(), log.Ctx{"saveVote": v.SubmittedBy.Hash})
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
