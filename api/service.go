package api

import (
	"net/http"
	"github.com/mariusor/littr.go/models"
	"github.com/mariusor/littr.go/app"
	ap "github.com/mariusor/activitypub.go/activitypub"
	json "github.com/mariusor/activitypub.go/jsonld"
		"github.com/go-chi/chi"
	"strings"
	"github.com/juju/errors"
	"fmt"
)

func loadAPService() *ap.Service {
	s := ap.ServiceNew(ap.ObjectID(fmt.Sprintf("%s", BaseURL, )))

	out := ap.OutboxNew()
	out.ID = ap.ObjectID(fmt.Sprintf("%s/%s", BaseURL, *out.GetID()))
	s.Outbox = out

	return s
}

// GET /api/outbox
func HandleServiceCollection(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error

	us := loadAPService()

	collection := chi.URLParam(r, "collection")
	switch strings.ToLower(collection) {
	case "inbox":
	case "outbox":
		items, err := models.LoadOPItems(Db, app.MaxContentItems)
		if err != nil {
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		_, err = loadAPCollection("/", us.Outbox, &items)
		data, err = json.WithContext(GetContext()).Marshal(us.Outbox)
	case "liked":
	default:
		err = errors.Errorf("collection not found")
	}
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
// GET /api/:collection/:hash
func HandleServiceCollectionItem(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error

	us := loadAPService()

	collection := chi.URLParam(r, "collection")
	var whichCol ap.CollectionInterface
	switch strings.ToLower(collection) {
	case "inbox":
		whichCol = us.Inbox
	case "outbox":
		whichCol = us.Outbox
	case "liked":
		whichCol = us.Liked
	default:
		err = errors.Errorf("collection not found")
	}
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}

	hash := chi.URLParam(r, "hash")
	c, err  := models.LoadItem(Db, hash)
	if err != nil {
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	el, _ := loadAPItem(c,"", whichCol)

	data, err = json.WithContext(GetContext()).Marshal(el)
	w.Header().Set("Content-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
