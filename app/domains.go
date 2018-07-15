package app

import (
	"log"
		"github.com/mariusor/littr.go/models"

		"github.com/gin-gonic/gin"
)

// handleMain serves /domains/{domain} request
func HandleDomains(c *gin.Context) {
	r := c.Request
	w := c.Writer
	vars := c.Params

	m := userModel{InvertedTheme: IsInverted}

	selC := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "content_items"."flags", "content_items"."metadata", "accounts"."handle" from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where substring(data::text from 'http[s]?://([^/]*)') = $1 order by "submitted_at" desc`
	{
		rows, err := Db.Query(selC, vars.ByName("domain"))
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

	RenderTemplate(r, w, "user.html", m)
}
