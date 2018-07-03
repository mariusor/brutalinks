package main

import (
	"github.com/gorilla/mux"
	"html/template"
	"log"
	"net/http"
)

// handleMain serves /domains/{domain} request
func (l *littr)handleDomains(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	db := l.Db
	m := userModel{}

	selC := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "content_items"."flags", "content_items"."metadata"  from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where substring(data::text from 'http[s]?://([^/]*)') = $1 order by "submitted_at" desc`
	{
		rows, err := db.Query(selC, vars["domain"])
		if err != nil {
			l.handleError(w, r, err, -1)
			return
		}
		for rows.Next() {
			p := Content{}
			err = rows.Scan(&p.id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.flags, &p.Metadata)
			if err != nil {
				l.handleError(w, r, err, -1)
				return
			}
			m.Items = append(m.Items, p)
		}
	}
	err := CurrentAccount().LoadVotes(getAllIds(m.Items))
	if err != nil {
		log.Print(err)
	}

	var t *template.Template
	var terr error
	t, terr = template.New("user.html").ParseFiles(templateDir + "user.html")
	if terr != nil {
		log.Print(terr)
	}
	t.Funcs(template.FuncMap{
		"formatDateInterval": relativeDate,
		"formatDate":         formatDate,
		"sluggify":           sluggify,
		"title":			  func(t []byte) string { return string(t) },
		"getProviders": 	  getAuthProviders,
		"CurrentAccount": 	  CurrentAccount,
		"LoadFlashMessages":  LoadFlashMessages,
		"CleanFlashMessages":  CleanFlashMessages,
	})
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("items.html").ParseFiles(templateDir + "partials/content/items.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("flash.html").ParseFiles(templateDir + "partials/flash.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("score.html").ParseFiles(templateDir + "partials/content/score.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("data.html").ParseFiles(templateDir + "partials/content/data.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("link.html").ParseFiles(templateDir + "partials/content/link.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("meta.html").ParseFiles(templateDir + "partials/content/meta.html")
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
	}
}