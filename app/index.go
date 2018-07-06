package app

import (
	"fmt"
	"log"

	"net/http"

	"github.com/mariusor/littr.go/models"
)

const (
	MaxContentItems = 200
)

type indexModel struct {
	Title         string
	InvertedTheme bool
	Items         []models.Content
}

func getAuthProviders() map[string]string {
	p := make(map[string]string)
	p["github"] = "Github"
	//p["gitlab"] = "Gitlab"
	//p["google"] = "Google"
	//p["facebook"] = "Facebook"

	return p
}

// handleMain serves / request
func (l *Littr) HandleIndex(w http.ResponseWriter, r *http.Request) {
	m := indexModel{Title: "Index", InvertedTheme: l.InvertedTheme}

	db := l.Db

	sel := fmt.Sprintf(`select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "submitted_by", "handle", "content_items"."flags" 
		from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
		where path is NULL
	order by "score" desc, "submitted_at" desc limit %d`, MaxContentItems)
	rows, err := db.Query(sel)
	if err != nil {
		l.HandleError(w, r, err, -1)
		return
	}
	for rows.Next() {
		p := models.Content{}
		err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Handle, &p.Flags)
		if err != nil {
			l.HandleError(w, r, err, -1)
			return
		}
		m.Items = append(m.Items, p)
	}

	err = l.LoadVotes(CurrentAccount, getAllIds(m.Items))
	if err != nil {
		log.Print(err)
	}

	err = l.SessionStore.Save(r, w, l.GetSession(r))
	if err != nil {
		log.Print(err)
	}

	t, terr := l.LoadTemplates(templateDir, "index.html")
	if terr != nil {
		log.Print(terr)
		return
	}
	terr = t.Execute(w, m)
	if terr != nil {
		log.Print(terr)
		return
	}
}
