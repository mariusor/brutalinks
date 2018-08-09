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
	loader, ok := val.(models.CanLoad)
	if ok {
		log.Infof("loaded LoaderService of type %T", loader)
	} else {
		log.Errorf("could not load loader service from Context")
		return
	}

	items, err := loader.LoadItems(models.LoadItemsFilter{MaxItems:app.MaxContentItems})
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
		//types := r.URL.Query()["type"]
		//clauses := make(models.Clauses, 0)
		//if types == nil {
		//	clauses = nil
		//} else {
		//	for _, typ := range types {
		//		if strings.ToLower(typ) == strings.ToLower(string(ap.LikeType)) {
		//			clauses = append(clauses, models.Clause{ColName: `"votes"."weight" > `, Val: interface{}(0)})
		//		} else {
		//			clauses = append(clauses, models.Clause{ColName: ` "votes"."weight" < `, Val: interface{}(0)})
		//		}
		//	}
		//}
		//
		//votes, err := models.LoadVotes(Db, clauses, app.MaxContentItems)
		//if err != nil {
		//	HandleError(w, r, http.StatusInternalServerError, err)
		//	return
		//}
		//_, err = loadAPLiked(us.Liked, votes)
		//data, err = json.WithContext(GetContext()).Marshal(us.Liked)
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

	//us := loadAPService()
	//collection := chi.URLParam(r, "collection")
	//var whichCol ap.CollectionInterface
	//switch strings.ToLower(collection) {
	//case "inbox":
	//	whichCol = us.Inbox
	//case "outbox":
	//	whichCol = us.Outbox
	//case "liked":
	//	whichCol = us.Liked
	//default:
	//	err = errors.Errorf("collection not found")
	//}
	//if err != nil {
	//	HandleError(w, r, http.StatusInternalServerError, err)
	//	return
	//}

	hash := chi.URLParam(r, "hash")
	c, err  := models.LoadItem(hash)
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
