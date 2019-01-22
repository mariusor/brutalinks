package api

import (
	"fmt"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/log"
	"io/ioutil"
	"net/http"

	"github.com/go-chi/chi"

	ap "github.com/mariusor/littr.go/app/activitypub"

	j "github.com/go-ap/jsonld"
)

// POST /api - not implemented yet - but we should have all information in the CreateActivity body
// PUT /api/actors/{handle}/{collection}/{item_hash}
func (h handler)UpdateItem(w http.ResponseWriter, r *http.Request) {
	var body []byte
	var err error
	defer r.Body.Close()
	status := http.StatusInternalServerError

	if body, err = ioutil.ReadAll(r.Body); err != nil {
		h.logger.WithContext(log.Ctx{
			"err":   err,
			"trace": errors.Details(err),
		}).Error("request body read error")
		h.HandleError(w, r, errors.NewNotValid(err, "not found"))
		return
	}

	location := ""
	col := chi.URLParam(r, "collection")
	switch col {
	case "outbox":
		act := ap.Activity{}
		if err := j.Unmarshal(body, &act); err != nil {
			h.logger.WithContext(log.Ctx{
				"err":   err,
				"trace": errors.Details(err),
			}).Error("json-ld unmarshal error")
			h.HandleError(w, r, errors.NewNotValid(err, "not found"))
			return
		}
		it := app.Item{}
		if err := it.FromActivityPub(act); err != nil {
			h.logger.WithContext(log.Ctx{
				"err":   err,
				"trace": errors.Details(err),
			}).Error("json-ld unmarshal error")
			h.HandleError(w, r, errors.NewNotValid(err, "not found"))
			return
		}
		if repo, ok := app.ContextItemSaver(r.Context()); ok {
			newIt, err := repo.SaveItem(it)
			if err != nil {
				h.logger.WithContext(log.Ctx{
					"err":     err,
					"trace":   errors.Details(err),
					"item":    it.Hash,
					"account": it.SubmittedBy.Hash,
				}).Error(err.Error())
				h.HandleError(w, r, errors.NewNotValid(err, "not found"))
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
			h.logger.WithContext(log.Ctx{
				"err":   err,
				"trace": errors.Details(err),
			}).Error("json-ld unmarshal error")
			h.HandleError(w, r, errors.NewNotValid(err, "not found"))
			return
		}
		v := app.Vote{}
		if err := v.FromActivityPub(act); err != nil {
			h.logger.WithContext(log.Ctx{
				"err":   err,
				"trace": errors.Details(err),
			}).Error("json-ld unmarshal error")
			h.HandleError(w, r, errors.NewNotValid(err, "not found"))
			return
		}
		if repo, ok := app.ContextVoteSaver(r.Context()); ok {
			newVot, err := repo.SaveVote(v)
			if err != nil {
				h.logger.WithContext(log.Ctx{
					"err":      err,
					"trace":    errors.Details(err),
					"saveVote": v.SubmittedBy.Hash,
				}).Error(err.Error())
				h.HandleError(w, r, errors.NewNotValid(err, "not found"))
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
