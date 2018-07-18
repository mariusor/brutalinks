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

var APIAccountsURL = BaseURL + "/accounts"

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

// GET /api/accounts/:handle
func HandleAccount(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")
	a, err := loadAccount(handle)
	if err != nil {
		log.Print(err)
		//HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	//HandleError(w, r, http.StatusNotFound, fmt.Errorf("acccount not found"))
	if a.Handle == "" {
		log.Print("could not load account information")
		return
	}

	p := ap.PersonNew(ap.ObjectID(a.Hash()))
	p.Name["en"] = a.Handle
	p.PreferredUsername["en"] = a.Handle
	p.URL = ap.URI(fmt.Sprintf(fmt.Sprintf("%s/%s", APIAccountsURL, a.Handle)))

	p.Outbox.URL = BuildObjectURL(p, p.Outbox)
	p.Inbox.URL = BuildObjectURL(p, p.Inbox)
	p.Liked.URL = BuildObjectURL(p, p.Liked)

	//json.Ctx = GetContext()
	j, err := json.Marshal(p)
	if err != nil {
		//HandleError(w, r, http.StatusInternalServerError, err)
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

	p := ap.PersonNew(ap.ObjectID(a.Hash()))
	p.Name["en"] = a.Handle
	p.PreferredUsername["en"] = a.Handle
	p.URL = ap.URI(fmt.Sprintf(fmt.Sprintf("%s/%s", APIAccountsURL, a.Handle)))

	//json.Ctx = GetContext()

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

		p.Outbox.URL = BuildObjectURL(p, p.Outbox)
		for _, item := range *items {
			note := ap.ObjectNew(ap.ObjectID(item.Hash()), ap.ArticleType)
			note.Content["en"] = string(item.Data)
			if item.Title != nil {
				note.Name["en"] = string(item.Title)
			}
			note.Published = item.SubmittedAt
			note.Updated = item.UpdatedAt
			note.URL = ap.URI(app.PermaLink(item, a.Handle))
			p.Outbox.Append(note)

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

	p := ap.PersonNew(ap.ObjectID(a.Hash()))
	p.Name["en"] = a.Handle
	p.PreferredUsername["en"] = a.Handle
	p.URL = ap.URI(a.GetLink())
	p.Outbox.URL = BuildObjectURL(p, p.Outbox)
	p.Inbox.URL = BuildObjectURL(p, p.Inbox)
	//json.Ctx = GetContext()
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
