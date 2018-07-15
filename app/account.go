package app

import (
	"fmt"
	"log"
	"net/http"

	"github.com/mariusor/littr.go/models"

	"github.com/gorilla/mux"
	"github.com/gin-gonic/gin"
)

type userModel struct {
	Title         string
	InvertedTheme func(r *http.Request) bool
	User          *Account
	Items         []Item
}

func HandleUser(c *gin.Context) {
	r := c.Request
	w := c.Writer
	vars := mux.Vars(r)

	m := userModel{InvertedTheme: IsInverted}

	found := false

	u := models.Account{}
	selAcct := `select "id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags" from "accounts" where "handle" = $1`
	{
		rows, err := Db.Query(selAcct, c.Params.ByName("handle"))
		if err != nil {
			HandleError(w, r, StatusUnknown, err)
			return
		}
		for rows.Next() {
			err = rows.Scan(&u.Id, &u.Key, &u.Handle, &u.Email, &u.Score, &u.CreatedAt, &u.UpdatedAt, &u.Metadata, &u.Flags)
			if err != nil {
				HandleError(w, r, StatusUnknown, err)
				return
			}
			found = true
		}
		m.Title = fmt.Sprintf("Activity %s", u.Handle)
		m.User = &Account{
			Id:        u.Id,
			Hash:      u.Hash(),
			flags:     u.Flags,
			UpdatedAt: u.UpdatedAt,
			Handle:    u.Handle,
			metadata:  u.Metadata,
			Score:     u.Score,
			CreatedAt: u.CreatedAt,
			Email:     u.Email,
		}
	}

	if !found {
		HandleError(w, r, http.StatusNotFound, fmt.Errorf("user %q not found", vars["handle"]))
		return
	}

	selC := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "content_items"."flags", "content_items"."metadata", "accounts"."handle" f from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where "submitted_by" = $1 order by "submitted_at" desc`
	{
		rows, err := Db.Query(selC, u.Id)
		if err != nil {
			HandleError(w, r, StatusUnknown, err)
			return
		}
		for rows.Next() {
			p := models.Content{}
			var handle string
			err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.Flags, &p.Metadata, &handle)
			if err != nil {
				HandleError(w, r, StatusUnknown, err)
				return
			}
			l := LoadItem(p, handle)
			m.Items = append(m.Items, l)
		}
	}
	_, err := LoadVotes(CurrentAccount, m.Items)
	if err != nil {
		log.Print(err)
	}
	err = SessionStore.Save(r, w, GetSession(r))
	if err != nil {
		log.Print(err)
	}

	RenderTemplate(r, w, "user.html", m)
}
