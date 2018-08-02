package api

import (
	"net/http"
	"fmt"
	"strings"
	"github.com/juju/errors"
	"github.com/go-chi/chi"
	log "github.com/sirupsen/logrus"
	ap "github.com/mariusor/activitypub.go/activitypub"
	json "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/models"
	"github.com/mariusor/littr.go/app"
)

var CurrentAccount *models.Account

func apObjectID(a models.Account) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, a.Handle))
}

func loadAPItem(item models.Content, handle string, o ap.CollectionInterface) (ap.Item, error) {
	if o == nil {
		return nil, errors.Errorf("unable to load item")
	}
	id := ap.ObjectID(fmt.Sprintf("%s/%s", *o.GetID(), item.Hash()))
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
		o.URL = ap.URI(app.PermaLink(item, handle))
		o.MediaType = ap.MimeType(item.MimeType)
		if item.Title != nil {
			o.Name["en"] = string(item.Title)
		}
		el = o
	}
	return el, nil
}

func loadLikedItems(id int64) (*[]models.Content, *[]models.Vote, error) {
	var err error
	items := make([]models.Content, 0)
	votes := make([]models.Vote, 0)
	selC := `select 
		"votes"."id", 
		"votes"."weight", 
		"votes"."submitted_at", 
		"votes"."flags",
		"content_items"."id", 
		"content_items"."key", 
		"content_items"."mime_type", 
		"content_items"."data", 
		"content_items"."title", 
		"content_items"."score",
		"content_items"."submitted_at", 
		"content_items"."submitted_by",
		"content_items"."flags", 
		"content_items"."metadata", 
		"accounts"."handle"
from "content_items"
  inner join "votes" on "content_items"."id" = "votes"."item_id"
  left join "accounts" on "accounts"."id" = "content_items"."submitted_by"
where "votes"."submitted_by" = $1 order by "votes"."submitted_at" desc`
	{
		rows, err := Db.Query(selC, id)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			v := models.Vote{}
			p := models.Content{}
			err = rows.Scan(
				&v.Id,
				&v.Weight,
				&v.SubmittedAt,
				&v.Flags,
				&p.Id,
				&p.Key,
				&p.MimeType,
				&p.Data,
				&p.Title,
				&p.Score,
				&p.SubmittedAt,
				&p.SubmittedBy,
				&p.Flags,
				&p.Metadata,
				&p.Handle)
			if err != nil {
				return nil, nil, err
			}
			v.SubmittedBy = id
			items = append(items, p)
			votes = append(votes, v)
		}
	}
	if err != nil {
		log.Print(err)
	}
	return &items, &votes, nil
}
func loadSubmittedItems(id int64) (*[]models.Content, error) {
	var err error
	items := make([]models.Content, 0)
	selC := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "content_items"."flags", "content_items"."metadata", "accounts"."handle" f from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where "submitted_by" = $1 order by "submitted_at" desc`
	{
		rows, err := Db.Query(selC, id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			p := models.Content{}
			err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.Flags, &p.Metadata, &p.Handle)
			if err != nil {
				return nil, err
			}
			p.SubmittedBy = id
			items = append(items, p)
		}
	}
	if err != nil {
		log.Print(err)
	}

	return &items, nil
}
func loadReceivedItems(id int64)(*[]models.Content, error) {
	return nil, nil
}

func loadAccount(handle string) (*models.Account, error) {
	a := models.Account{}
	selAcct := `select "id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags" from "accounts" where "handle" = $1`
	rows, err := Db.Query(selAcct, handle)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		err = rows.Scan(&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.UpdatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return nil, err
		}
	}

	if err != nil {
		log.Print(err)
	}

	return &a, nil
}

func loadAPPerson(a models.Account) *ap.Person {
	baseURL := ap.URI(fmt.Sprintf("%s", AccountsURL))

	p := ap.PersonNew(ap.ObjectID(apObjectID(a)))
	p.Name["en"] = a.Handle
	p.PreferredUsername["en"] = a.Handle

	out := ap.OutboxNew()
	out.URL = BuildObjectURL(p.URL, p.Outbox)
	out.ID = BuildObjectID("", p, p.Outbox)
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

func loadAPLiked(a *models.Account, o ap.CollectionInterface, items *[]models.Content, votes *[]models.Vote) (ap.CollectionInterface, error) {
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

		typ :=  ap.ArticleType
		if item.IsLink() {
			typ =  ap.LinkType
		}
		oid := ap.ObjectID(fmt.Sprintf("%s/%s/outbox/%s", AccountsURL, item.Handle, item.Hash()))
		obj := ap.ObjectNew(oid, typ)
		obj.URL = ap.URI(fmt.Sprintf("%s/%s", a.GetLink(), item.Hash()))

		id := ap.ObjectID(fmt.Sprintf("%s/%s", *o.GetID(), item.Hash()))
		var it ap.Item
		if vote.Weight > 0 {
			l := ap.LikeNew(id, obj)
			l.Published = vote.SubmittedAt
			l.Updated = item.UpdatedAt
			it = l
		} else  {
			d := ap.DislikeNew(id, obj)
			d.Published = vote.SubmittedAt
			d.Updated = item.UpdatedAt
			it = d
		}

		o.Append(it)
	}

	return o, nil
}

func loadAPCollection(a *models.Account, o ap.CollectionInterface, items *[]models.Content) (ap.CollectionInterface, error) {
	if items == nil || len(*items) == 0 {
		return nil, errors.Errorf("empty collection %T", o)
	}
	for _, item := range *items {
		el, _ := loadAPItem(item, a.Handle, o)

		//oc := ap.OrderedCollection(p.Outbox)
		//pag := ap.OrderedCollectionPageNew(&oc)
		o.Append(el)
	}

	return o, nil
}

// GET /api/accounts/:handle
func HandleAccount(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")
	a, err := loadAccount(handle)
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
	p := loadAPPerson(*a)

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
	a, err := loadAccount(chi.URLParam(r, "handle"))
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("acccount not found"))
		return
	}

	p := loadAPPerson(*a)

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
	sel := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score",
			"submitted_at", "submitted_by", "path", "content_items"."flags" from "content_items"
			where "content_items"."key" ~* $1`
	rows, err := Db.Query(sel, hash)
	if err != nil {
		return
	}

	var c models.Content
	var el ap.Item
	for rows.Next() {
		err = rows.Scan(&c.Id, &c.Key, &c.MimeType, &c.Data, &c.Title, &c.Score, &c.SubmittedAt, &c.SubmittedBy, &c.Path, &c.Flags)
		if err != nil {
			return
		}
		el, _ = loadAPItem(c, a.Handle, whichCol)
	}

	data, err = json.WithContext(GetContext()).Marshal(el)
	w.Header().Set("Content-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
// GET /api/accounts/:handle/:collection
func HandleAccountCollection(w http.ResponseWriter, r *http.Request) {
	var data []byte
	a, err := loadAccount(chi.URLParam(r, "handle"))
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("acccount not found"))
		return
	}

	p:= loadAPPerson(*a)

	collection := chi.URLParam(r, "collection")
	switch strings.ToLower(collection) {
	case "inbox":
		items, err := loadReceivedItems(a.Id)
		if err != nil {
			log.Print(err)
		}
		_, err = loadAPCollection(a, p.Inbox, items)
		data, err = json.Marshal(p.Inbox)
	case "outbox":
		items, err := loadSubmittedItems(a.Id)
		if err != nil {
			log.Print(err)
		}
		_, err = loadAPCollection(a, p.Outbox, items)
		data, err = json.Marshal(p.Outbox)
	case "liked":
		items, votes, err := loadLikedItems(a.Id)
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
	a := CurrentAccount
	if a == nil {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("acccount not found"))
		return
	}

	a, err := loadAccount(a.Handle)
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("acccount not found"))
		return
	}

	p := loadAPPerson(*a)

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
