package api

import (
	"net/http"

	"github.com/mariusor/littr.go/models"

	"fmt"

	"github.com/gorilla/mux"
	ap "github.com/mariusor/activitypub.go/activitypub"
	json "github.com/mariusor/activitypub.go/jsonld"
)

var CurrentAccount *models.Account

func loadAccount(handle string) (*models.Account, int64, error) {
	a := models.Account{}
	var itemCount int64
	selAcct := `select "accounts"."id", "accounts"."key", "handle", "email", "accounts"."score", "created_at", "accounts"."updated_at",
  "accounts"."metadata", "accounts"."flags",
  count("content_items"."id") as "submission_count" from "accounts"
left join "content_items" on "content_items"."submitted_by" = "accounts"."id"
 where "handle" = $1 group by "accounts"."id"`
	rows, err := Db.Query(selAcct, handle)
	if err != nil {
		return nil, 0, err
	}
	for rows.Next() {
		err = rows.Scan(&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.UpdatedAt, &a.Metadata, &a.Flags, &itemCount)
		if err != nil {
			return nil, 0, err
		}
	}
	return &a, itemCount, nil
}

// GET /api/accounts/{handle}
func HandleAccount(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	a, _, err := loadAccount(vars["handle"])
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
