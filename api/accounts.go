package api

import (
	"fmt"
	"github.com/go-chi/chi"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	json "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strings"
)

var CurrentAccount *models.Account

func getObjectID(s string) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, s))
}

func apAccountID(a models.Account) ap.ObjectID {
	return getObjectID(a.Handle)
}

func loadAPItem(item models.Content, o ap.CollectionInterface) (ap.Item, error) {
	if o == nil {
		return nil, errors.Errorf("unable to load item")
	}

	id := BuildObjectIDFromContent(item)
	var el ap.Item
	if item.IsLink() {
		l := ap.LinkNew(id, ap.LinkType)
		l.Href = ap.URI(item.Data)
		el = l
	} else {
		o := ap.ObjectNew(id, ap.ArticleType)
		o.Content["en"] = string(item.Data)
		o.Published = item.SubmittedAt
		o.Updated = item.UpdatedAt
		o.URL = ap.URI(app.PermaLink(item, item.SubmittedByAccount.Handle))
		o.MediaType = ap.MimeType(item.MimeType)
		if item.Title != nil {
			o.Name["en"] = string(item.Title)
		}
		el = o
	}
	return el, nil
}


func loadReceivedItems(id int64) (*[]models.Content, error) {
	return nil, nil
}

func loadAPPerson(a models.Account) *ap.Person {
	baseURL := ap.URI(fmt.Sprintf("%s", AccountsURL))

	p := ap.PersonNew(ap.ObjectID(apAccountID(a)))
	p.Name["en"] = a.Handle
	p.PreferredUsername["en"] = a.Handle

	out := ap.OutboxNew()
	out.URL = BuildObjectURL(p.URL, p.Outbox)
	out.ID = BuildCollectionID(a, p.Outbox)
	p.Outbox = out
	/*
		in := ap.InboxNew()
		in.URL = BuildObjectURL(p.URL, p.Inbox)
		in.ID = BuildObjectID("", p,  p.Inbox)
		p.Inbox = in

		liked := ap.LikedNew()
		liked.URL = BuildObjectURL(p.URL, p.Liked)
		liked.ID = BuildObjectID("", p, p.Liked)
		p.Liked = liked
	*/

	p.URL = BuildObjectURL(baseURL, p)

	return p
}

func loadAPLiked(a models.Account, o ap.CollectionInterface, items *[]models.Content, votes *[]models.Vote) (ap.CollectionInterface, error) {
	if items == nil || len(*items) == 0 {
		return nil, errors.Errorf("no items loaded")
	}
	if votes == nil || len(*votes) == 0 {
		return nil, errors.Errorf("no votes loaded")
	}
	if len(*items) != len(*votes) {
		return nil, errors.Errorf("items and votes lengths are not matching")
	}
	for k, item := range *items {
		vote := (*votes)[k]
		if vote.Weight == 0 {
			// skip 0 weight votes from the collection
			continue
		}

		typ := ap.ArticleType
		if item.IsLink() {
			typ = ap.LinkType
		}
		oid := ap.ObjectID(fmt.Sprintf("%s/%s/outbox/%s", AccountsURL, item.SubmittedByAccount.Handle, item.Hash()))
		obj := ap.ObjectNew(oid, typ)
		obj.URL = ap.URI(fmt.Sprintf("%s/%s", a.GetLink(), item.Hash()))

		id := ap.ObjectID(fmt.Sprintf("%s/%s", *o.GetID(), item.Hash()))
		var it ap.Item
		if vote.Weight > 0 {
			l := ap.LikeNew(id, obj)
			l.Published = vote.SubmittedAt
			l.Updated = item.UpdatedAt
			it = l
		} else {
			d := ap.DislikeNew(id, obj)
			d.Published = vote.SubmittedAt
			d.Updated = item.UpdatedAt
			it = d
		}

		o.Append(it)
	}

	return o, nil
}

func loadAPCollection(o ap.CollectionInterface, a ap.Item, items *[]models.Content) (ap.CollectionInterface, error) {
	if items == nil || len(*items) == 0 {
		return nil, errors.Errorf("empty collection %T", o)
	}
	for _, item := range *items {
		el, _ := loadAPItem(item, o)

		o.Append(el)
	}

	return o, nil
}

// GET /api/accounts/:handle
func HandleAccount(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")
	a, err := models.LoadAccount(Db, handle)
	if err != nil {
		log.Print(err)
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusInternalServerError, err)
		log.Print("could not load account information")
		return
	}
	p := loadAPPerson(a)

	j, err := json.WithContext(GetContext()).Marshal(p)
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		log.Print(err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}

// GET /api/accounts/:handle/:collection/:hash
func HandleAccountCollectionItem(w http.ResponseWriter, r *http.Request) {
	var data []byte
	a, err := models.LoadAccount(Db, chi.URLParam(r, "handle"))
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("acccount not found"))
		return
	}

	p := loadAPPerson(a)

	collection := chi.URLParam(r, "collection")
	var whichCol ap.CollectionInterface
	switch strings.ToLower(collection) {
	case "inbox":
		whichCol = p.Inbox
	case "outbox":
		whichCol = p.Outbox
	case "liked":
		whichCol = p.Liked
	default:
		err = errors.Errorf("collection not found")
	}

	hash := chi.URLParam(r, "hash")
	c, err  := models.LoadItem(Db, hash)
	if err != nil {
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	el, _ := loadAPItem(c, whichCol)

	data, err = json.WithContext(GetContext()).Marshal(el)
	w.Header().Set("Content-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/accounts/:handle/:collection
func HandleAccountCollection(w http.ResponseWriter, r *http.Request) {
	var data []byte
	a, err := models.LoadAccount(Db, chi.URLParam(r, "handle"))
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("acccount not found"))
		return
	}

	p := loadAPPerson(a)

	collection := chi.URLParam(r, "collection")
	switch strings.ToLower(collection) {
	case "inbox":
		items, err := loadReceivedItems(a.Id)
		if err != nil {
			log.Print(err)
		}
		_, err = loadAPCollection(p.Inbox, p, items)
		data, err = json.WithContext(GetContext()).Marshal(p.Inbox)
	case "outbox":
		items, err := models.LoadItemsSubmittedBy(Db, a.Handle)
		if err != nil {
			log.Print(err)
		}
		_, err = loadAPCollection(p.Outbox, p, &items)
		data, err = json.WithContext(GetContext()).Marshal(p.Outbox)
	case "liked":
		items, votes, err := models.LoadItemsAndVotesSubmittedBy(Db, a.Handle)
		if err != nil {
			log.Print(err)
		}
		_, err = loadAPLiked(a, p.Liked, items, votes)
		data, err = json.WithContext(GetContext()).Marshal(p.Liked)
	default:
		err = errors.Errorf("collection not found")
	}

	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/accounts/verify_credentials
func HandleVerifyCredentials(w http.ResponseWriter, r *http.Request) {
	a := *CurrentAccount
	if len(a.Handle) == 0 {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("acccount not found"))
		return
	}

	a, err := models.LoadAccount(Db, a.Handle)
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("acccount not found"))
		return
	}

	p := loadAPPerson(a)

	j, err := json.WithContext(GetContext()).Marshal(p)
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}
