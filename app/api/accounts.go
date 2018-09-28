package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/mariusor/littr.go/app/frontend"

	"github.com/go-chi/chi"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	json "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

func getObjectID(s string) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, s))
}

func apAccountID(a models.Account) ap.ObjectID {
	if len(a.Hash) < 9 {
		return ap.ObjectID("::unknown")
	}
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, a.Hash.String()))
}

func loadAPLike(vote models.Vote) ap.ObjectOrLink {
	if vote.Weight == 0 {
		return nil
	}
	id := BuildObjectIDFromItem(*vote.Item)
	lID := BuildObjectIDFromVote(vote)
	whomArt := ap.IRI(BuildActorHashID(*vote.SubmittedBy))
	if vote.Weight > 0 {
		l := ap.LikeNew(lID, ap.IRI(id))
		l.AttributedTo = whomArt
		return *l
	} else {
		l := ap.DislikeNew(lID, ap.IRI(id))
		l.AttributedTo = whomArt
		return *l
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
	o.MediaType = MimeType(item.MimeType)
	o.Generator = ap.IRI("http://littr.git")
	o.Score = item.Score / models.ScoreMultiplier
	if item.Title != "" {
		o.Name.Set("en", string(item.Title))
	}
	if item.Data != "" {
		o.Content.Set("en", string(item.Data))
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
	p.Type = ActivityVocabularyType(ap.PersonType)
	p.Name = ap.NaturalLanguageValueNew()
	p.PreferredUsername = ap.NaturalLanguageValueNew()

	if len(a.Hash) > 7 {
		p.ID = apAccountID(a)
	}

	p.Name.Set("en", a.Handle)
	p.PreferredUsername.Set("en", a.Handle)

	out := ap.OutboxNew()
	p.Outbox = out
	out.ID = BuildCollectionID(a, p.Outbox)
	out.URL = BuildObjectURL(p.URL, p.Outbox)
	if len(a.Handle) > 0 {
		out.AttributedTo = ap.URI(p.ID)
	}
	//in := ap.InboxNew()
	//p.Inbox = in
	//in.ID = BuildCollectionID(a, p.Inbox)
	//in.URL = BuildObjectURL(p.URL, p.Inbox)

	liked := ap.LikedNew()
	p.Liked = liked
	liked.ID = BuildCollectionID(a, p.Liked)
	liked.URL = BuildObjectURL(p.URL, p.Liked)
	if len(a.Handle) > 0 {
		liked.AttributedTo = ap.URI(p.ID)
	}

	p.URL = BuildObjectURL(baseURL, p)
	p.Score = a.Score
	if a.IsValid() && a.HasMetadata() && a.Metadata.Key != nil && a.Metadata.Key.Public != nil {
		p.PublicKey = PublicKey{
			Id:           ObjectID(fmt.Sprintf("%s#main-key", p.ID)),
			Owner:        ap.IRI(p.ID),
			PublicKeyPem: fmt.Sprintf("-----BEGIN PUBLIC KEY-----\n%s\n-----END PUBLIC KEY-----", base64.StdEncoding.EncodeToString(a.Metadata.Key.Public)),
		}
	}
	return &p
}

func loadAPLiked(o ap.CollectionInterface, votes models.VoteCollection) (ap.CollectionInterface, error) {
	if votes == nil || len(votes) == 0 {
		return nil, errors.Errorf("empty collection %T", o)
	}
	for _, vote := range votes {
		el := loadAPLike(vote)

		if el != nil {
			o.Append(el)
		}
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

// GET /api/accounts?filters
func HandleAccountsCollection(w http.ResponseWriter, r *http.Request) {
	var ok bool
	var filter models.LoadAccountsFilter
	var data []byte

	f := r.Context().Value(FilterCtxtKey)
	if filter, ok = f.(models.LoadAccountsFilter); !ok {
		log.WithFields(log.Fields{}).Errorf("could not load filter from Context")
		return
	} else {
		val := r.Context().Value(RepositoryCtxtKey)
		if service, ok := val.(models.CanLoadAccounts); ok {
			var accounts models.AccountCollection
			var err error

			col := ap.Collection{}
			if accounts, err = service.LoadAccounts(filter); err == nil {
				for _, acct := range accounts {
					col.Append(loadAPPerson(acct))
				}
				data, err = json.WithContext(GetContext()).Marshal(col)
			} else {
				log.WithFields(log.Fields{"trace": errors.Trace(err)}).Error(err)
			}
		}
	}
	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/accounts/:handle
func HandleAccount(w http.ResponseWriter, r *http.Request) {
	val := r.Context().Value(AccountCtxtKey)
	a, _ := val.(models.Account)
	//if !ok {
	//	log.WithFields(log.Fields{}).Errorf("could not load Account from Context")
	//}
	p := loadAPPerson(a)

	j, err := json.WithContext(GetContext()).Marshal(p)
	if err != nil {
		log.WithFields(log.Fields{"trace": errors.Trace(err)}).Error(err)
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/activity+json")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
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
			log.WithFields(log.Fields{}).Errorf("could not load Item from Context")
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		el, err = loadAPItem(i)
		if err != nil {
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
		val := r.Context().Value(RepositoryCtxtKey)
		if service, ok := val.(models.CanLoadItems); ok && len(i.Hash) > 0 {
			replies, err := service.LoadItems(models.LoadItemsFilter{
				InReplyTo: []string{i.Hash.String()},
				MaxItems:  MaxContentItems,
			})
			if err != nil {
				log.WithFields(log.Fields{"trace": errors.Trace(err)}).Error(err)
			}
			if len(replies) > 0 {
				if o, ok := el.(Article); ok {
					o.Replies = ap.CollectionNew(BuildRepliesCollectionID(o))
					el = o
				}
			}
		}
	case "liked":
		if v, ok := val.(models.Vote); !ok {
			log.WithFields(log.Fields{}).Errorf("could not load Vote from Context")
		} else {
			el = loadAPLike(v)
		}
	default:
		log.WithFields(log.Fields{}).Error(errors.Errorf("collection not found"))
	}

	data, err = json.WithContext(GetContext()).Marshal(el)
	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
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
		log.WithFields(log.Fields{}).Errorf("could not load Item from Context")
		return
	} else {
		val := r.Context().Value(RepositoryCtxtKey)
		el, err := loadAPItem(it)
		if service, ok := val.(models.CanLoadItems); ok {
			var replies models.ItemCollection

			filter := models.LoadItemsFilter{
				InReplyTo: []string{it.Hash.String()},
				MaxItems:  MaxContentItems,
			}

			p, _ := el.(Article)
			p.Replies = ap.CollectionNew(BuildRepliesCollectionID(p))
			if replies, err = service.LoadItems(filter); err == nil {
				_, err = loadAPCollection(p.Replies, &replies)
				data, err = json.WithContext(GetContext()).Marshal(p.Replies)
			} else {
				log.WithFields(log.Fields{}).Error(err)
			}
		}
	}

	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/accounts/:handle/:collection
// GET /api/:collection
func HandleCollection(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error
	val := r.Context().Value(AccountCtxtKey)
	a, _ := val.(models.Account)
	//if !ok {
	//	log.WithFields(log.Fields{}).WithFields(log.Fields{}).Errorf("could not load Account from Context")
	//}
	p := loadAPPerson(a)

	typ := chi.URLParam(r, "collection")

	collection := r.Context().Value(CollectionCtxtKey)
	switch strings.ToLower(typ) {
	case "inbox":
	case "outbox":
		items, ok := collection.(models.ItemCollection)
		if !ok {
			log.WithFields(log.Fields{}).Errorf("could not load Items from Context")
			return
		}
		_, err = loadAPCollection(p.Outbox, &items)
		data, err = json.WithContext(GetContext()).Marshal(p.Outbox)
	case "liked":
		votes, ok := collection.(models.VoteCollection)
		if !ok {
			log.WithFields(log.Fields{}).Errorf("could not load Votes from Context")
			return
		}
		if err != nil {
			log.WithFields(log.Fields{}).Error(err)
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

	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/accounts/verify_credentials
func HandleVerifyCredentials(w http.ResponseWriter, r *http.Request) {
	acct := frontend.CurrentAccount
	if acct == nil || len(acct.Handle) == 0 {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("account not found"))
		return
	}
	val := r.Context().Value(RepositoryCtxtKey)
	AcctLoader, ok := val.(models.CanLoadAccounts)
	if !ok {
		log.WithFields(log.Fields{}).Errorf("could not load account repository from Context")
	}
	a, err := AcctLoader.LoadAccount(models.LoadAccountsFilter{Handle: []string{acct.Handle}, MaxItems: 1})
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
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
	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}
