package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/mariusor/littr.go/app"

	"github.com/mariusor/littr.go/app/frontend"

	"github.com/go-chi/chi"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	as "github.com/mariusor/activitypub.go/activitystreams"
	json "github.com/mariusor/activitypub.go/jsonld"
	localap "github.com/mariusor/littr.go/app/activitypub"
	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

func getObjectID(s string) as.ObjectID {
	return as.ObjectID(fmt.Sprintf("%s/%s", ActorsURL, s))
}

func apAccountID(a models.Account) as.ObjectID {
	if len(a.Hash) >= 8 {
		return as.ObjectID(fmt.Sprintf("%s/%s", ActorsURL, a.Hash.String()))
	}
	return as.ObjectID(fmt.Sprintf("%s/anonymous", ActorsURL))
}

func loadAPLike(vote models.Vote) as.ObjectOrLink {
	if vote.Weight == 0 {
		return nil
	}
	id, _ := BuildObjectIDFromItem(*vote.Item)
	lID := BuildObjectIDFromVote(vote)
	whomArt := as.IRI(BuildActorHashID(*vote.SubmittedBy))
	if vote.Weight > 0 {
		l := as.LikeNew(lID, as.IRI(id))
		l.AttributedTo = whomArt
		return *l
	} else {
		l := as.DislikeNew(lID, as.IRI(id))
		l.AttributedTo = whomArt
		return *l
	}
}

func loadAPActivity(it models.Item) as.Activity {
	a := loadAPPerson(*it.SubmittedBy)
	ob := loadAPItem(it)

	obID := string(*ob.GetID())

	act := as.Activity{
		Type:      as.CreateType,
		ID:        as.ObjectID(fmt.Sprintf("%s", strings.Replace(obID, "/object", "", 1))),
		Published: it.SubmittedAt,
		To:        as.ItemCollection{as.IRI("https://www.w3.org/ns/activitystreams#Public")},
		CC:        as.ItemCollection{as.IRI(BuildGlobalOutboxID())},
	}

	act.Object = ob
	act.Actor = a.GetLink()

	return act
}

func loadAPItem(item models.Item) as.Item {
	o := localap.Article{}
	o.Name = make(as.NaturalLanguageValue, 0)
	if id, ok := BuildObjectIDFromItem(item); ok {
		o.ID = id
	}

	if item.MimeType == models.MimeTypeURL {
		o.Type = as.PageType
		o.URL = as.IRI(item.Data)
	} else {
		wordCount := strings.Count(item.Data, " ") +
			strings.Count(item.Data, "\t") +
			strings.Count(item.Data, "\n") +
			strings.Count(item.Data, "\r\n")
		if wordCount > 300 {
			o.Type = as.ArticleType
		} else {
			o.Type = as.NoteType
		}
		if len(item.Hash) > 0 {
			o.URL = as.IRI(frontend.ItemPermaLink(item))
		}
		o.MediaType = as.MimeType(item.MimeType)
		o.Content = make(as.NaturalLanguageValue, 0)
		if item.Data != "" {
			o.Content.Set("en", string(item.Data))
		}
	}
	o.Published = item.SubmittedAt
	o.Updated = item.UpdatedAt

	//o.Generator = as.IRI(app.Instance.BaseURL)
	o.Score = item.Score / models.ScoreMultiplier
	if item.Title != "" {
		o.Name.Set("en", string(item.Title))
	}
	if item.SubmittedBy != nil {
		id := BuildActorID(*item.SubmittedBy)
		handle := strings.Replace(string(id), item.SubmittedBy.Hash.String(), item.SubmittedBy.Handle, 1)
		o.AttributedTo = as.IRI(handle)
	}
	if item.Parent != nil {
		id, _ := BuildObjectIDFromItem(*item.Parent)
		o.InReplyTo = as.IRI(id)
	}
	if item.OP != nil {
		id, _ := BuildObjectIDFromItem(*item.OP)
		o.Context = as.IRI(id)
	}
	return o
}

func loadAPPerson(a models.Account) *localap.Person {
	p := localap.Person{}
	p.Type = as.PersonType
	p.Name = as.NaturalLanguageValueNew()
	p.PreferredUsername = as.NaturalLanguageValueNew()

	if a.Metadata != nil {
		if a.Metadata.Blurb != nil && len(a.Metadata.Blurb) > 0 {
			p.Summary = as.NaturalLanguageValueNew()
			p.Summary.Set(as.NilLangRef, string(a.Metadata.Blurb))
		}
		if a.Metadata.Avatar.Path != nil && len(a.Metadata.Avatar.Path) > 0 {
			avatar := as.ObjectNew(as.ImageType)
			avatar.MediaType = as.MimeType(a.Metadata.Avatar.MimeType)
			avatar.URL = as.IRI(a.Metadata.Avatar.Path)
			p.Icon = avatar
		}
	}

	if len(a.Hash) >= 8 {
		p.ID = apAccountID(a)
	}

	p.Name.Set("en", a.Handle)
	p.PreferredUsername.Set("en", a.Handle)

	out := ap.OutboxNew()
	out.ID = BuildCollectionID(a, new(ap.Outbox))
	p.Outbox = out

	in := ap.InboxNew()
	in.ID = BuildCollectionID(a, new(ap.Inbox))
	p.Inbox = in

	liked := ap.LikedNew()
	liked.ID = BuildCollectionID(a, new(ap.Liked))
	p.Liked = liked

	p.URL = as.IRI(frontend.AccountPermaLink(a))
	p.Score = a.Score
	if a.IsValid() && a.HasMetadata() && a.Metadata.Key != nil && a.Metadata.Key.Public != nil {
		p.PublicKey = localap.PublicKey{
			ID:           as.ObjectID(fmt.Sprintf("%s#main-key", p.ID)),
			Owner:        as.IRI(p.ID),
			PublicKeyPem: fmt.Sprintf("-----BEGIN PUBLIC KEY-----\n%s\n-----END PUBLIC KEY-----", base64.StdEncoding.EncodeToString(a.Metadata.Key.Public)),
		}
	}
	return &p
}

func loadAPLiked(o as.CollectionInterface, votes models.VoteCollection) (as.CollectionInterface, error) {
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

func loadAPCollection(o as.CollectionInterface, items *models.ItemCollection) (as.CollectionInterface, error) {
	if items == nil || len(*items) == 0 {
		return nil, errors.Errorf("empty collection %T", o)
	}
	for _, item := range *items {
		o.Append(loadAPActivity(item))
	}

	return o, nil
}

// HandleActorsCollection is the http handler for the actors collection
// GET /api/actors?filters
func HandleActorsCollection(w http.ResponseWriter, r *http.Request) {
	var ok bool
	var filter models.LoadAccountsFilter
	var data []byte

	f := r.Context().Value(models.FilterCtxtKey)
	if filter, ok = f.(models.LoadAccountsFilter); !ok {
		Logger.WithFields(log.Fields{}).Errorf("could not load filter from Context")
		HandleError(w, r, http.StatusNotFound, errors.New("not found"))
		return
	} else {
		val := r.Context().Value(models.RepositoryCtxtKey)
		if service, ok := val.(models.CanLoadAccounts); ok {
			var accounts models.AccountCollection
			var err error

			col := as.CollectionNew(as.ObjectID(ActorsURL))
			if accounts, err = service.LoadAccounts(filter); err == nil {
				for _, acct := range accounts {
					p := loadAPPerson(acct)
					p.Inbox = p.Inbox.GetLink()
					p.Outbox = p.Outbox.GetLink()
					p.Liked = p.Liked.GetLink()
					col.Append(p)
				}
				if len(accounts) > 0 {
					fpUrl := string(*col.GetID()) + "?page=1"
					col.First = as.IRI(fpUrl)
				}
				data, err = json.WithContext(GetContext()).Marshal(col)
			} else {
				Logger.WithFields(log.Fields{"trace": errors.Trace(err)}).Error(err)
				HandleError(w, r, http.StatusNotFound, err)
				return
			}
		}
	}
	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/actors/:handle
func HandleActor(w http.ResponseWriter, r *http.Request) {
	val := r.Context().Value(AccountCtxtKey)

	var ok bool
	var a models.Account
	if a, ok = val.(models.Account); !ok {
		Logger.WithFields(log.Fields{}).Errorf("could not load Account from Context")
	}
	p := loadAPPerson(a)
	if p.Outbox != nil {
		p.Outbox = p.Outbox.GetLink()
	}
	if p.Liked != nil {
		p.Liked = p.Liked.GetLink()
	}
	if p.Inbox != nil {
		p.Inbox = p.Inbox.GetLink()
	}
	p.Endpoints = as.Endpoints{SharedInbox: as.IRI(fmt.Sprintf("%s/api/self/inbox", app.Instance.BaseURL))}

	j, err := json.WithContext(GetContext()).Marshal(p)
	if err != nil {
		Logger.WithFields(log.Fields{"trace": errors.Trace(err)}).Error(err)
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/activity+json")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}

func getCollectionFromReq( r *http.Request) string {
	collection := chi.URLParam(r, "collection")
	if path.Base(r.URL.Path) == "replies" {
		collection = "replies"
	}
	return collection
}

// GET /api/actors/:handle/:collection/:hash
// GET /api/:collection/:hash
func HandleCollectionActivity(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error

	val := r.Context().Value(ItemCtxtKey)
	collection := getCollectionFromReq(r)
	var el as.ObjectOrLink
	switch strings.ToLower(collection) {
	case "replies":
		fallthrough
	case "inbox":
		fallthrough
	case "outbox":
		item, ok := val.(models.Item)
		if !ok {
			err := errors.New("could not load Item from Context")
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		el = loadAPActivity(item)
		if err != nil {
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
	case "liked":
		if v, ok := val.(models.Vote); !ok {
			err := errors.Errorf("could not load Vote from Context")
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		} else {
			el = loadAPLike(v)
		}
	default:
		err := errors.Errorf("collection %s not found", collection)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}

	data, err = json.WithContext(GetContext()).Marshal(el)
	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/actors/:handle/:collection/:hash/object
// GET /api/:collection/:hash/object
func HandleCollectionActivityObject(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error

	val := r.Context().Value(ItemCtxtKey)
	collection := chi.URLParam(r, "collection")
	var el as.ObjectOrLink
	switch strings.ToLower(collection) {
	case "inbox":
	case "replies":
	case "outbox":
		i, ok := val.(models.Item)
		if !ok {
			Logger.WithFields(log.Fields{}).Errorf("could not load Item from Context")
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		el = loadAPItem(i)
		if err != nil {
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
		val := r.Context().Value(models.RepositoryCtxtKey)
		if service, ok := val.(models.CanLoadItems); ok && len(i.Hash) > 0 {
			replies, err := service.LoadItems(models.LoadItemsFilter{
				InReplyTo: []string{i.Hash.String()},
				MaxItems:  MaxContentItems,
			})
			if err != nil {
				Logger.WithFields(log.Fields{"trace": errors.Trace(err)}).Error(err)
			}
			if len(replies) > 0 {
				if o, ok := el.(localap.Article); ok {
					o.Replies = as.IRI(BuildRepliesCollectionID(o))
					el = o
				}
			}
		}
	case "liked":
		if v, ok := val.(models.Vote); !ok {
			err := errors.Errorf("could not load Vote from Context")
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		} else {
			el = loadAPLike(v)
		}
	default:
		err := errors.Errorf("collection %s not found", collection)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}

	data, err = json.WithContext(GetContext()).Marshal(el)
	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/self/:collection
// GET /api/actors/:handle/:collection
// GET /api/self/:collection/:hash/replies
// GET /api/actors/:handle/:collection/:hash/replies
func HandleCollection(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error

	val := r.Context().Value(AccountCtxtKey)
	a, _ := val.(models.Account)
	typ := getCollectionFromReq(r)
	filters := r.Context().Value(models.FilterCtxtKey)

	f, _ := filters.(models.LoadItemsFilter)

	collection := r.Context().Value(CollectionCtxtKey)
	switch strings.ToLower(typ) {
	case "inbox":
		items, ok := collection.(models.ItemCollection)
		if !ok {
			err := errors.New("could not load Items from Context")
			Logger.WithFields(log.Fields{}).Error(err)
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
		col := ap.InboxNew()
		col.ID = BuildCollectionID(a, new(ap.Inbox))
		_, err = loadAPCollection(col, &items)
		if len(items) > 0 {
			url := fmt.Sprintf("%s?page=%d", string(*col.GetID()), f.Page)
			col.First = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", 1), 1))
			if f.Page > 0 {
				oc := as.OrderedCollection(*col)
				page := as.OrderedCollectionPageNew(&oc)
				page.ID = as.ObjectID(url)
				page.Next = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", f.Page+1), 1))
				if f.Page > 1 {
					page.Prev = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", f.Page-1), 1))
				}
			}
		}
		data, err = json.WithContext(GetContext()).Marshal(col)
	case "outbox":
		items, ok := collection.(models.ItemCollection)
		if !ok {
			err := errors.New("could not load Items from Context")
			Logger.WithFields(log.Fields{}).Error(err)
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
		col := ap.OutboxNew()
		col.ID = BuildCollectionID(a, new(ap.Outbox))
		_, err = loadAPCollection(col, &items)
		if len(items) > 0 {
			url := fmt.Sprintf("%s?page=%d", string(*col.GetID()), f.Page)
			col.First = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", 1), 1))
			if f.Page > 0 {
				oc := as.OrderedCollection(*col)
				page := as.OrderedCollectionPageNew(&oc)
				page.ID = as.ObjectID(url)
				page.Next = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", f.Page+1), 1))
				if f.Page > 1 {
					page.Prev = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", f.Page-1), 1))
				}
			}
		}
		data, err = json.WithContext(GetContext()).Marshal(col)
	case "liked":
		votes, ok := collection.(models.VoteCollection)
		if !ok {
			err := errors.New("could not load Votes from Context")
			Logger.WithFields(log.Fields{}).Error(err)
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
		if err != nil {
			Logger.WithFields(log.Fields{}).Error(err)
		}
		liked := ap.LikedNew()
		liked.ID = BuildCollectionID(a, new(ap.Likes))
		_, err = loadAPLiked(liked, votes)
		if len(votes) > 0 {
			url := fmt.Sprintf("%s?page=%d", string(*liked.GetID()), f.Page)
			liked.First = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", 1), 1))
			if f.Page > 0 {
				oc := as.OrderedCollection(*liked)
				page := as.OrderedCollectionPageNew(&oc)
				page.ID = as.ObjectID(url)
				page.Next = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", f.Page+1), 1))
				if f.Page > 1 {
					page.Prev = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", f.Page-1), 1))
				}
			}
		}
		data, err = json.WithContext(GetContext()).Marshal(liked)
	case "replies":
		it, ok := r.Context().Value(ItemCtxtKey).(models.Item)
		var art as.Item
		if ok {
			art = loadAPItem(it)
		}
		items, ok := collection.(models.ItemCollection)
		if !ok {
			err := errors.New("could not load Replies from Context")
			Logger.WithFields(log.Fields{}).Error(err)
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
		replies := localap.OrderedCollectionNew(as.ObjectID(""))
		replies.ID = BuildRepliesCollectionID(art)
		_, err = loadAPCollection(replies, &items)
		if len(items) > 0 {
			url := fmt.Sprintf("%s?page=%d", string(*replies.GetID()), f.Page)
			replies.First = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", 1), 1))
			if f.Page > 0 {
				oc := as.OrderedCollection(*replies)
				page := as.OrderedCollectionPageNew(&oc)
				page.ID = as.ObjectID(url)
				page.Next = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", f.Page+1), 1))
				if f.Page > 1 {
					page.Prev = as.IRI(strings.Replace(url, fmt.Sprintf("page=%d", f.Page), fmt.Sprintf("page=%d", f.Page-1), 1))
				}
			}
		}
		data, err = json.WithContext(GetContext()).Marshal(replies)
	default:
		err = errors.Errorf("collection %s not found", typ)
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

// GET /api/actors/verify_credentials
func HandleVerifyCredentials(w http.ResponseWriter, r *http.Request) {
	acct, ok := models.ContextCurrentAccount(r.Context())
	if !ok {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("account not found"))
		return
	}
	AcctLoader, ok := models.ContextAccountLoader(r.Context())
	if !ok {
		Logger.WithFields(log.Fields{}).Errorf("could not load account repository from Context")
	}
	a, err := AcctLoader.LoadAccount(models.LoadAccountsFilter{Handle: []string{acct.Handle}, MaxItems: 1})
	if err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
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
