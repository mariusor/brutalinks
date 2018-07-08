package api

import (
	"log"
	"net/http"

	"github.com/mariusor/littr.go/models"

	"fmt"

	"github.com/gorilla/mux"
	ap "github.com/mariusor/activitypub.go/activitypub"
	json "github.com/mariusor/activitypub.go/jsonld"
)

var CurrentAccount *models.Account

func loadAccount(handle string) (*models.Account, *[]models.Content, error) {
	a := models.Account{}
	selAcct := `select "id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags" from "accounts" where "handle" = $1`
	rows, err := Db.Query(selAcct, handle)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		err = rows.Scan(&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.UpdatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return nil, nil, err
		}
	}

	items := make([]models.Content, 0)
	selC := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "content_items"."flags", "content_items"."metadata", "accounts"."handle" f from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where "submitted_by" = $1 order by "submitted_at" desc`
	{
		rows, err := Db.Query(selC, a.Id)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			p := models.Content{}
			err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.Flags, &p.Metadata, &p.Handle)
			if err != nil {
				return nil, nil, err
			}
			p.SubmittedBy = a.Id
			items = append(items, p)
		}
	}
	if err != nil {
		log.Print(err)
	}

	return &a, &items, nil
}

// GET /api/accounts/{handle}
func HandleAccount(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	a, items, err := loadAccount(vars["handle"])
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, fmt.Errorf("acccount not found"))
		return
	}

	p := ap.PersonNew(ap.ObjectID(a.Hash()))
	p.Name["en"] = a.Handle
	p.PreferredUsername["en"] = a.Handle
	p.URL = fmt.Sprintf(a.PermaLink())

	p.Outbox = ap.OutboxStream(ap.Outbox(ap.OrderedCollection{ID: ap.ObjectID("outbox")}))
	for _, item := range *items {
		note := ap.ObjectNew(ap.ObjectID(item.Hash()), ap.ArticleType)
		note.Content["en"] = string(item.Data)
		if item.Title != nil {
			note.Name["en"] = string(item.Title)
		}
		note.Published = item.SubmittedAt
		note.Updated = item.UpdatedAt
		note.URL = item.PermaLink()
		p.Outbox.Append(note)
	}

	json.Ctx = GetContext()
	j, err := json.Marshal(p)
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
	return

}

// GET /api/accounts/verify_credentials
func HandleVerifyCredentials(w http.ResponseWriter, r *http.Request) {
	a := CurrentAccount
	if a == nil {
		HandleError(w, r, http.StatusNotFound, fmt.Errorf("acccount not found"))
	}

	a, _, err := loadAccount(a.Handle)
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, http.StatusNotFound, fmt.Errorf("acccount not found"))
		return
	}

	p := ap.PersonNew(ap.ObjectID(a.Hash()))
	p.Name["en"] = a.Handle
	p.PreferredUsername["en"] = a.Handle
	p.URL = fmt.Sprintf(a.PermaLink())

	json.Ctx = GetContext()
	j, err := json.Marshal(p)
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}
