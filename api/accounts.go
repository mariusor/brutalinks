package api

import (
	"log"
	"net/http"

	"github.com/mariusor/littr.go/models"

	"fmt"

	"strings"
		"github.com/go-chi/chi"
	ap "github.com/mariusor/activitypub.go/activitypub"
	json "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/app"
)

var CurrentAccount *models.Account

func apObjectID(a models.Account) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, a.Handle))
}

func loadItems(id int64) (*[]models.Content, error) {
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

	log.Printf("loading person %s", baseURL)
	p := ap.PersonNew(ap.ObjectID(apObjectID(a)))
	p.Name["en"] = a.Handle
	p.PreferredUsername["en"] = a.Handle
	p.URL = BuildObjectURL(baseURL, p)

	p.Outbox.URL = BuildObjectURL(p.URL, p.Outbox)
	p.Outbox.ID = BuildObjectID("", p, p.Outbox)
	p.Inbox.URL = BuildObjectURL(p.URL, p.Inbox)
	p.Inbox.ID = BuildObjectID("", p, p.Inbox)
	p.Liked.URL = BuildObjectURL(p.URL, p.Liked)
	p.Liked.ID = BuildObjectID("", p, p.Liked)

	return p
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

	json.Ctx = GetContext()
	j, err := json.Marshal(p)
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

// GET /api/accounts/:handle/:path
func HandleAccountPath(w http.ResponseWriter, r *http.Request) {
	var data []byte
	a, err := loadAccount(chi.URLParam(r, "handle"))
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, fmt.Errorf("acccount not found"))
		return
	}

	p:= loadAPPerson(*a)
	json.Ctx = GetContext()

	path := chi.URLParam(r, "path")
	switch strings.ToLower(path) {
	case "outbox":
		items, err := loadItems(a.Id)
		if err != nil {
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
		if a.Handle == "" {
			HandleError(w, r, http.StatusNotFound, fmt.Errorf("acccount not found"))
			return
		}

		p.Outbox.ID = BuildObjectID(r.Host, p, p.Outbox)
		p.Outbox.URL = BuildObjectURL(p.URL, p.Outbox)
		for _, item := range *items {
			id := ap.ObjectID(fmt.Sprintf("%s/%s", p.Outbox.GetID(), item.Hash()))
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
				o.URL = ap.URI(app.PermaLink(item, a.Handle))
				o.MediaType = ap.MimeType(item.MimeType)
				if item.Title != nil {
					o.Name["en"] = string(item.Title)
				}
				el = o
			}

			//oc := ap.OrderedCollection(p.Outbox)
			//pag := ap.OrderedCollectionPageNew(&oc)
			p.Outbox.Append(el)

			data, err = json.Marshal(p.Outbox)
		}
	case "inbox":
		data, err = json.Marshal(p.Inbox)
	case "liked":
		data, err = json.Marshal(p.Liked)
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
		HandleError(w, r, http.StatusNotFound, fmt.Errorf("acccount not found"))
		return
	}

	a, err := loadAccount(a.Handle)
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, fmt.Errorf("acccount not found"))
		return
	}

	p := loadAPPerson(*a)

	json.Ctx = GetContext()
	j, err := json.Marshal(p)
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}
