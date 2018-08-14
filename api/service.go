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
	log "github.com/sirupsen/logrus"
)

func loadAPService() *ap.Service {
	s := ap.ServiceNew(ap.ObjectID(fmt.Sprintf("%s", BaseURL, )))

	out := ap.OutboxNew()
	out.ID = ap.ObjectID(fmt.Sprintf("%s/%s", BaseURL, *out.GetID()))
	s.Outbox = out
	liked := ap.LikedNew()
	out.ID = ap.ObjectID(fmt.Sprintf("%s/%s", BaseURL, *liked.GetID()))
	s.Liked = liked

	return s
}

// GET /api/outbox
func HandleServiceCollection(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error

	us := loadAPService()
	val := r.Context().Value(ServiceCtxtKey)
	loader, ok := val.(models.CanLoadItems)
	if ok {
		log.Infof("loaded LoaderService of type %T", loader)
	} else {
		log.Errorf("could not load loader service from Context")
		return
	}

	types := []models.ItemType{models.TypeOP} // ap.ArticleType
	items, err := loader.LoadItems(models.LoadItemsFilter{Type: types, MaxItems:app.MaxContentItems})
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	collection := chi.URLParam(r, "collection")
	switch strings.ToLower(collection) {
	case "inbox":
	case "outbox":
		_, err = loadAPCollection(us.Outbox, &items)
		if err != nil {
			log.Error(err)
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
		data, err = json.WithContext(GetContext()).Marshal(us.Outbox)
		if err != nil {
			log.Error(err)
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
	case "liked":
		types := r.URL.Query()["type"]
		var which []models.VoteType
		if types == nil {
			which =  []models.VoteType{
				models.VoteType(strings.ToLower(string(ap.LikeType))),
				models.VoteType(strings.ToLower(string(ap.DislikeType))),
			}
		} else {
			for _, typ := range types {
				if strings.ToLower(typ) == strings.ToLower(string(ap.LikeType)) {
					which = []models.VoteType{models.VoteType(strings.ToLower(string(ap.LikeType))),}
				} else {
					which = []models.VoteType{models.VoteType(strings.ToLower(string(ap.DislikeType))),}
				}
			}
		}
		votes, err := models.Service.LoadVotes(models.LoadVotesFilter{
			Type: which,
			MaxItems: app.MaxContentItems,
		})
		if err != nil {
			log.Print(err)
		}
		_, err = loadAPLiked(us.Liked, votes)
		data, err = json.WithContext(GetContext()).Marshal(us.Liked)
	default:
		err = errors.Errorf("collection not found")
	}
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("item-Type", "application/json; charset=utf-8")
	w.Header().Set("X-item-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
// GET /api/:collection/:hash
func HandleServiceCollectionItem(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error

	hash := chi.URLParam(r, "hash")
	c, err  := models.Service.LoadItem(models.LoadItemsFilter{Key: []string{hash}})
	if err != nil {
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	el, _ := loadAPItem(c)

	data, err = json.WithContext(GetContext()).Marshal(el)
	w.Header().Set("item-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-item-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
