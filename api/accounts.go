package api

import (
	"fmt"
	"github.com/go-chi/chi"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	json "github.com/mariusor/activitypub.go/jsonld"
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

//func loadAPLike(vote models.Vote) (ap.ObjectOrLink, error) {
//	id := BuildObjectIDFromItem(*vote.Item)
//	lID := BuildObjectIDFromVote(vote)
//	whomArt := ap.IRI(BuildActorID(*vote.Item.SubmittedByAccount))
//	if vote.Weight > 0 {
//		l := ap.LikeNew(lID, ap.IRI(id))
//		l.AttributedTo = whomArt
//		return *l, nil
//	} else {
//		l := ap.DislikeNew(lID, ap.IRI(id))
//		l.AttributedTo = whomArt
//		return *l, nil
//	}
//}

func loadAPItem(item models.Item) (ap.Item, error) {
	id := BuildObjectIDFromItem(item)
	o := Article{}
	o.Name = make(ap.NaturalLanguageValue, 0)
	o.Content = make(ap.NaturalLanguageValue, 0)
	o.ID = ObjectID(id)
	o.Type = ActivityVocabularyType(ap.ArticleType)
	o.Published = item.SubmittedAt
	o.Updated = item.UpdatedAt
	//o.URL = ap.URI(PermaLink(item))
	o.MediaType = MimeType(item.MimeType)
	o.Generator = ap.IRI("http://littr.git")
	o.Score =  item.Score/models.ScoreMultiplier
	if item.Title != "" {
		o.Name["en"] = string(item.Title)
	}
	if item.Data != "" {
		o.Content["en"] = string(item.Data)
	}
	if item.SubmittedBy != nil {
		o.AttributedTo = ap.IRI(BuildActorID(*item.SubmittedBy))
	}
	if item.Parent != nil {
		o.InReplyTo = ap.IRI(BuildObjectIDFromItem(*item.Parent))
	}
	if item.OP != nil {
		o.Context = ap.IRI(BuildObjectIDFromItem(*item.OP))
	}
	return o, nil
}

func loadReceivedItems(hash string) (*models.ItemCollection, error) {
	return nil, nil
}

func loadAPPerson(a models.Account) *Person {
	baseURL := ap.URI(fmt.Sprintf("%s", AccountsURL))

	p := Person{}
	p.Name = make(NaturalLanguageValue)
	p.PreferredUsername = make(NaturalLanguageValue)
	p.ID = ObjectID(ap.ObjectID(apAccountID(a)))
	p.Name["en"] = a.Handle
	p.PreferredUsername["en"] = a.Handle

	out := ap.OutboxNew()
	p.Outbox = out
	out.ID = BuildCollectionID(a, p.Outbox)
	out.URL = BuildObjectURL(p.URL, p.Outbox)
	out.AttributedTo = ap.URI(p.ID)

	in := ap.InboxNew()
	p.Inbox = in
	in.ID = BuildCollectionID(a,  p.Inbox)
	in.URL = BuildObjectURL(p.URL, p.Inbox)
	in.AttributedTo = ap.URI(p.ID)

	liked := ap.LikedNew()
	p.Liked = liked
	liked.ID = BuildCollectionID(a, p.Liked)
	liked.URL = BuildObjectURL(p.URL, p.Liked)
	liked.AttributedTo = ap.URI(p.ID)

	p.URL = BuildObjectURL(baseURL, p)
	p.Score = a.Score

	return &p
}

//func loadAPLiked(a models.account, o ap.CollectionInterface, items *[]models.item, votes *[]models.Vote) (ap.CollectionInterface, error) {
//	if items == nil || len(*items) == 0 {
//		return nil, errors.Errorf("no items loaded")
//	}
//	if votes == nil || len(*votes) == 0 {
//		return nil, errors.Errorf("no votes loaded")
//	}
//	if len(*items) != len(*votes) {
//		return nil, errors.Errorf("items and votes lengths are not matching")
//	}
//	for k, item := range *items {
//		vote := (*votes)[k]
//		if vote.Weight == 0 {
//			// skip 0 weight votes from the collection
//			continue
//		}
//
//		typ := ap.ArticleType
//		if item.IsLink() {
//			typ = ap.LinkType
//		}
//		oid := ap.ObjectID(fmt.Sprintf("%s/%s/outbox/%s", AccountsURL, item.SubmittedByAccount.Handle, item.Hash()))
//		obj := ap.ObjectNew(oid, typ)
//		obj.URL = ap.URI(fmt.Sprintf("%s/%s", a.GetLink(), item.Hash()))
//
//		id := ap.ObjectID(fmt.Sprintf("%s/%s", *o.GetID(), item.Hash()))
//		var it ap.Item
//		if vote.Weight > 0 {
//			l := ap.LikeNew(id, obj)
//			l.Published = vote.SubmittedAt
//			l.Updated = item.UpdatedAt
//			it = l
//		} else {
//			d := ap.DislikeNew(id, obj)
//			d.Published = vote.SubmittedAt
//			d.Updated = item.UpdatedAt
//			it = d
//		}
//
//		o.Append(it)
//	}
//
//	return o, nil
//}
//func loadAPLiked(o ap.CollectionInterface, votes *[]models.Vote) (ap.CollectionInterface, error) {
//	if votes == nil || len(*votes) == 0 {
//		return nil, errors.Errorf("empty collection %T", o)
//	}
//	for _, vote := range *votes {
//		el, _ := loadAPLike(vote)
//
//		o.Append(el)
//	}
//
//	return o, nil
//}
func loadAPCollection(o ap.CollectionInterface, items *models.ItemCollection) (ap.CollectionInterface, error) {
	if items == nil || len(*items) == 0 {
		return nil, errors.Errorf("empty collection %T", o)
	}
	for _, item := range *items {
		el, _ := loadAPItem(item)

		o.Append(el)
	}

	return o, nil
}

// GET /api/accounts/:handle
func HandleAccount(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")
	a, err := models.LoadAccount(handle)
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

	w.Header().Set("item-Type", "application/json; charset=utf-8")
	w.Header().Set("X-item-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}

// GET /api/accounts/:handle/:collection/:hash
func HandleAccountCollectionItem(w http.ResponseWriter, r *http.Request) {
	var data []byte
	a, err := models.LoadAccount(chi.URLParam(r, "handle"))
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("acccount not found"))
		return
	}

	//p := loadAPPerson(a)
	//
	//collection := chi.URLParam(r, "collection")
	//var whichCol ap.CollectionInterface
	//switch strings.ToLower(collection) {
	//case "inbox":
	//	whichCol = p.Inbox
	//case "outbox":
	//	whichCol = p.Outbox
	//case "liked":
	//	whichCol = p.Liked
	//default:
	//	err = errors.Errorf("collection not found")
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

// GET /api/accounts/:handle/:collection
func HandleAccountCollection(w http.ResponseWriter, r *http.Request) {
	var data []byte
	a, err := models.LoadAccount(chi.URLParam(r, "handle"))
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
		items, err := loadReceivedItems(a.Hash)
		if err != nil {
			log.Print(err)
		}
		_, err = loadAPCollection(p.Inbox, items)
		data, err = json.WithContext(GetContext()).Marshal(p.Inbox)
	case "outbox":
		items, err := models.LoadItemsSubmittedBy(a.Handle)
		if err != nil {
			log.Print(err)
		}
		_, err = loadAPCollection(p.Outbox, &items)
		data, err = json.WithContext(GetContext()).Marshal(p.Outbox)
	case "liked":
		//types := r.URL.Query()["type"]
		//var which int
		//if types == nil {
		//	which = 0
		//} else {
		//	for _, typ := range types {
		//		if strings.ToLower(typ) == strings.ToLower(string(ap.LikeType)) {
		//			which = 1
		//		} else {
		//			which = -1
		//		}
		//	}
		//}
		//votes, err := models.LoadVotesSubmittedBy(Db, a.Handle, which, app.MaxContentItems)
		//if err != nil {
		//	log.Print(err)
		//}
		//_, err = loadAPLiked(p.Liked, votes)
		//data, err = json.WithContext(GetContext()).Marshal(p.Liked)
	default:
		err = errors.Errorf("collection not found")
	}

	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("item-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-item-Type-Options", "nosniff")
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

	a, err := models.LoadAccount(a.Handle)
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
	w.Header().Set("item-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-item-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}
