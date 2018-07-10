package app

import (
	"log"
	"net/http"

	"github.com/mariusor/littr.go/models"

	"github.com/gorilla/mux"
)

// handleMain serves /domains/{domain} request
func (l *Littr) HandleDomains(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	db := l.Db
	m := userModel{InvertedTheme: IsInverted}

	selC := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "content_items"."flags", "content_items"."metadata", "accounts"."handle" from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where substring(data::text from 'http[s]?://([^/]*)') = $1 order by "submitted_at" desc`
	{
		rows, err := db.Query(selC, vars["domain"])
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

		_, err = LoadVotes(CurrentAccount, m.Items)
		if err != nil {
			log.Print(err)
		}
	}

	RenderTemplate(w, "user.html", m)
}
