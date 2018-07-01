package main

import (
	"net/http"
	"html/template"
	"math"
	"log"
	"github.com/astaxie/beego/orm"
	"fmt"
	"time"
	"net/url"
)

type newModel struct {
	Title   string
	Content Content
}

func detectMimeType(data []byte) string {
	u, err := url.ParseRequestURI(string(data))
	if err == nil && u != nil {
		return MimeTypeURL
	}
	return "text/plain"
}

// handleMain serves /{year}/{month}/{day}/{hash} request
func (l *littr) handleSubmit(w http.ResponseWriter, r *http.Request) {
	p := Content{}
	m := newModel{Title: "Submit new content", Content: p}
	db, err := orm.GetDB("default")
	if err != nil {
		l.handleError(w, r, err, -1)
		return
	}
	var userId int64 = 1

	if r.Method == http.MethodPost {
		p.Title = []byte(r.PostFormValue("title"))
		p.Data = []byte(r.PostFormValue("data"))
		if len(p.Data) > 0 {
			now := time.Now()
			p.MimeType = detectMimeType(p.Data)
			p.SubmittedAt = now
			p.UpdatedAt = now
			p.submittedBy = userId
			p.Key = p.GetKey()

			ins := `insert into "content_items" ("key", "title", "data", "mime_type", "submitted_by", "submitted_at", "updated_at") values($1, $2, $3, $4, $5, $6, $7)`
			{
				res, err := db.Exec(ins, p.Key, p.Title, p.Data, p.MimeType, p.submittedBy, p.SubmittedAt, p.UpdatedAt)
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

	var terr error
	var t *template.Template
	t, terr = template.New("new.html").ParseFiles(templateDir + "new.html")
	if terr != nil {
		log.Print(terr)
	}

	t.Funcs(template.FuncMap{
		"formatDateInterval": relativeDate,
		"formatDate":         formatDate,
		"sluggify":           sluggify,
		"title":			  func(t []byte) string { return string(t) },
		"mod":			  	  func(lvl int) float64 { return math.Mod(float64(lvl), float64(10)) },
		"getProviders": 	  getAuthProviders,
		"CurrentAccount": 	  CurrentAccount,
	})
	_, terr = t.New("submit.html").ParseFiles(templateDir + "partials/content/submit.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("head.html").ParseFiles(templateDir + "partials/head.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("header.html").ParseFiles(templateDir + "partials/header.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("footer.html").ParseFiles(templateDir + "partials/footer.html")
	if terr != nil {
		log.Print(terr)
	}
	terr = t.Execute(w, m)
	if terr != nil {
		log.Print(terr)
		return
	}
}