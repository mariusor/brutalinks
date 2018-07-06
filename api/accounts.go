package api

import (
	"net/http"
	"time"

	"github.com/mariusor/littr.go/models"

	"fmt"

	"github.com/gorilla/mux"
	ap "github.com/mariusor/activitypub.go/activitypub"
	json "github.com/mariusor/activitypub.go/jsonld"
)

type Account struct {
	//The username of the account
	Username string `json:"username"`
	// Equals username for local users, includes @domain for remote ones
	Acct string `json:"acct"`
	// The time the account was created
	CreatedAt time.Time `json:"created_at"`
	// URL of the user's profile page (can be remote)
	Url string `json:"url"`
	// The number of votes received
	Score int64 `json:"score"`
	//The number of statuses the account has made
	SubmissionsCount int64 `json:"submissions_count"`
	// Array of profile metadata field, each element has 'name' and 'value'
	Fields Fields `json:"fields"`
	// Boolean to indicate that the account performs automated actions
	Bot bool `json:"bot"`
}

var CurrentAccount *models.Account

func loadAccount(handle string) (*models.Account, int64, error) {
	a := models.Account{}
	var itemCount int64
	selAcct := `select "accounts"."id", "accounts"."key", "handle", "email", "accounts"."score", "created_at", "accounts"."updated_at",
  "accounts"."metadata", "accounts"."flags",
  count("content_items"."id") as "submission_count" from "accounts"
inner join "content_items" on "content_items"."submitted_by" = "accounts"."id"
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
		HandleError(w, r, err, http.StatusInternalServerError)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, fmt.Errorf("acccount not found"), http.StatusNotFound)
		return
	}

	u := ap.PersonNew(ap.ObjectID(a.Hash()))
	u.Name["en"] = a.Handle
	u.PreferredUsername["en"] = a.Handle
	u.URL = fmt.Sprintf(a.PermaLink())

	ctx := json.Context{}
	j, err := json.Marshal(u, &ctx)
	if err != nil {
		HandleError(w, r, err, http.StatusInternalServerError)
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
		HandleError(w, r, fmt.Errorf("acccount not found"), http.StatusNotFound)
	}

	a, items, err := loadAccount(a.Handle)
	if err != nil {
		HandleError(w, r, err, http.StatusInternalServerError)
		return
	}
	if a.Handle == "" {
		HandleError(w, r, fmt.Errorf("acccount not found"), http.StatusNotFound)
		return
	}

	u := Account{
		Username:         a.Handle,
		Acct:             a.Handle,
		CreatedAt:        a.CreatedAt,
		Url:              fmt.Sprintf("/~%s", a.Handle),
		Score:            a.Score,
		SubmissionsCount: items,
		Bot:              true,
	}

	ctx := json.Context{}
	j, err := json.Marshal(u, &ctx)
	if err != nil {
		HandleError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}
