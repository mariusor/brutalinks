package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	json "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
)

func getObjectID(s string) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, s))
}

func apAccountID(a models.Account) ap.ObjectID {
	return getObjectID(a.Handle)
}

func loadAPLike(vote models.Vote) (ap.ObjectOrLink, error) {
	id := BuildObjectIDFromItem(*vote.Item)
	lID := BuildObjectIDFromVote(vote)
	whomArt := ap.IRI(BuildActorID(*vote.SubmittedBy))
	if vote.Weight > 0 {
		l := ap.LikeNew(lID, ap.IRI(id))
		l.AttributedTo = whomArt
		return *l, nil
	} else {
		l := ap.DislikeNew(lID, ap.IRI(id))
		l.AttributedTo = whomArt
		return *l, nil
	}
}

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
	o.Score = item.Score / models.ScoreMultiplier
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
	in.ID = BuildCollectionID(a, p.Inbox)
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

func loadAPLiked(o ap.CollectionInterface, votes models.VoteCollection) (ap.CollectionInterface, error) {
	if votes == nil || len(votes) == 0 {
		return nil, errors.Errorf("empty collection %T", o)
	}
	for _, vote := range votes {
		el, _ := loadAPLike(vote)

		o.Append(el)
	}

	return o, nil
}

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
	val := r.Context().Value(AccountCtxtKey)
	a, ok := val.(models.Account)
	if !ok {
		log.Errorf("could not load Account from Context")
	}
	p := loadAPPerson(a)

	j, err := json.WithContext(GetContext()).Marshal(p)
	if err != nil {
		log.Print(err)
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("item-Type", "application/json; charset=utf-8")
	w.Header().Set("X-item-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}

// GET /api/accounts/:handle/:collection/:hash
// GET /api/:collection/:hash
func HandleCollectionItem(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error

	val := r.Context().Value(ItemCtxtKey)
	collection := chi.URLParam(r, "collection")
	var el ObjectOrLink
	switch strings.ToLower(collection) {
	case "inbox":
	case "outbox":
		i, ok := val.(models.Item)
		if !ok {
			log.Errorf("could not load Item from Context")
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		el, err = loadAPItem(i)
		if err != nil {
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
		val := r.Context().Value(ServiceCtxtKey)
		if service, ok := val.(models.CanLoadItems); ok {
			replies, err := service.LoadItems(models.LoadItemsFilter{
				InReplyTo: []string{i.Hash},
				MaxItems:  MaxContentItems,
			})
			if err != nil {
				log.Error(err)
			}
			if len(replies) > 0 {
				if o, ok := el.(Article); ok {
					o.Replies = ap.CollectionNew(BuildRepliesCollectionID(o))
					el = o
				}
			}
		}
	case "liked":
		v, ok := val.(models.Vote)
		if !ok {
			log.Errorf("could not load Vote from Context")
			return
		}
		el, err = loadAPLike(v)
		if err != nil {
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
	default:
		log.Error(errors.Errorf("collection not found"))
	}

	data, err = json.WithContext(GetContext()).Marshal(el)
	log.Error(err)
	w.Header().Set("item-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-item-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/accounts/:handle/:collection/:hash/replies
// GET /api/:collection/:hash/replies
func HandleItemReplies(w http.ResponseWriter, r *http.Request) {
	var ok bool

	var it models.Item
	item := r.Context().Value(ItemCtxtKey)
	var data []byte
	if it, ok = item.(models.Item); !ok {
		log.Errorf("could not load Item from Context")
		return
	} else {
		val := r.Context().Value(ServiceCtxtKey)
		el, err := loadAPItem(it)
		if service, ok := val.(models.CanLoadItems); ok {
			var replies models.ItemCollection

			filter := models.LoadItemsFilter{
				InReplyTo: []string{it.Hash},
				MaxItems:  MaxContentItems,
			}

			p, _ := el.(Article)
			p.Replies = ap.CollectionNew(BuildRepliesCollectionID(p))
			if replies, err = service.LoadItems(filter); err == nil {
				_, err = loadAPCollection(p.Replies, &replies)
				data, err = json.WithContext(GetContext()).Marshal(p.Replies)
			} else {
				log.Error(err)
			}
		}
	}

	w.Header().Set("item-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-item-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/accounts/:handle/:collection
// GET /api/:collection
func HandleCollection(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error
	val := r.Context().Value(AccountCtxtKey)
	a, ok := val.(models.Account)
	if !ok {
		log.Errorf("could not load Account from Context")
	}
	p := loadAPPerson(a)

	typ := chi.URLParam(r, "collection")

	collection := r.Context().Value(CollectionCtxtKey)
	switch strings.ToLower(typ) {
	case "inbox":
	case "outbox":
		items, ok := collection.(models.ItemCollection)
		if !ok {
			log.Errorf("could not load Items from Context")
			return
		}
		_, err = loadAPCollection(p.Outbox, &items)
		data, err = json.WithContext(GetContext()).Marshal(p.Outbox)
	case "liked":
		votes, ok := collection.(models.VoteCollection)
		if !ok {
			log.Errorf("could not load Votes from Context")
			return
		}
		if err != nil {
			log.Print(err)
		}
		_, err = loadAPLiked(p.Liked, votes)
		data, err = json.WithContext(GetContext()).Marshal(p.Liked)
	case "replies":
		items, ok := collection.(models.ItemCollection)
		if !ok {
			log.Errorf("could not load Replies from Context")
			return
		}
		_, err = loadAPCollection(p.Replies, &items)
		data, err = json.WithContext(GetContext()).Marshal(p.Outbox)
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
	acct := app.CurrentAccount
	if acct == nil || len(acct.Handle) == 0 {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("account not found"))
		return
	}
	val := r.Context().Value(ServiceCtxtKey)
	AcctLoader, ok := val.(models.CanLoadAccounts)
	if ok {
		log.Infof("loaded LoaderService of type %T", AcctLoader)
	} else {
		log.Errorf("could not load account loader service from Context")
	}
	a, err := AcctLoader.LoadAccount(models.LoadAccountFilter{Handle: acct.Handle})
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("account not found"))
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
