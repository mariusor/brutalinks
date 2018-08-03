package app

import (
	log "github.com/sirupsen/logrus"
	"net/http"

	"github.com/mariusor/littr.go/models"

	"fmt"
	"github.com/go-chi/chi"
)

// HandleDomains serves /domains/{domain} request
func HandleDomains(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")

	items := make([]Item, 0)
	m := userModel{ Title: fmt.Sprintf("Submissions from %s", domain), InvertedTheme: IsInverted(r)}
	selC := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "content_items"."flags", "content_items"."metadata", "accounts"."handle" from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where substring(data::text from 'http[s]?://([^/]*)') = $1 order by "submitted_at" desc`
	{
		rows, err := Db.Query(selC, domain)
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
			com := comment{Item: l, Path: p.Path, FullPath: p.FullPath()}
			items = append(items, l)
			m.Items = append(m.Items, com)
		}

		_, err = LoadVotes(CurrentAccount, items)
		if err != nil {
			log.Print(err)
		}
	}

	RenderTemplate(r, w, "listing", m)
}
