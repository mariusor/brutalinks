package main

import (
	"fmt"
	"log"
	"models"
	"net/http"
	"net/url"
	"time"
)

type newModel struct {
	Title         string
	InvertedTheme bool
	Content       models.Content
}

func detectMimeType(data []byte) string {
	u, err := url.ParseRequestURI(string(data))
	if err == nil && u != nil {
		return models.MimeTypeURL
	}
	return "text/plain"
}

// handleMain serves /{year}/{month}/{day}/{hash} request
func (l *littr) handleSubmit(w http.ResponseWriter, r *http.Request) {
	p := models.Content{}
	m := newModel{Title: "Submit new content", Content: p}
	db := l.Db
	var userId = CurrentAccount().Id

	if r.Method == http.MethodPost {
		p.Title = []byte(r.PostFormValue("title"))
		p.Data = []byte(r.PostFormValue("data"))
		if len(p.Data) > 0 {
			now := time.Now()
			p.MimeType = detectMimeType(p.Data)
			p.SubmittedAt = now
			p.UpdatedAt = now
			p.SubmittedBy = userId
			p.Key = p.GetKey()

			ins := `insert into "content_items" ("key", "title", "data", "mime_type", "submitted_by", "submitted_at", "updated_at") values($1, $2, $3, $4, $5, $6, $7)`
			{
				res, err := db.Exec(ins, p.Key, p.Title, p.Data, p.MimeType, p.SubmittedBy, p.SubmittedAt, p.UpdatedAt)
				if err != nil {
					log.Print(err)
				} else {
					if rows, _ := res.RowsAffected(); rows == 0 {
						log.Print(fmt.Errorf("could not save new reply %q", p.Hash()))
					}
				}
			}
		}
		l.Vote(p, 1, userId)
		http.Redirect(w, r, p.PermaLink(), http.StatusMovedPermanently)
	}

	err := l.session.Save(r, w, l.Session(r))
	if err != nil {
		log.Print(err)
	}

	t, terr := l.LoadTemplates(templateDir, "new.html")
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
