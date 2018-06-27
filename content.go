package main

import (
	"net/http"
	"html/template"
	"github.com/gorilla/mux"
	"github.com/astaxie/beego/orm"
	"fmt"
	"time"
	"strings"
	"log"
)

type contentModel struct {
	Title string
	Content Content
}
const (
	MimeTypeTextPlain = "text/plain"
	MimeTypeHtml = "text/html"
	MimeTypeMarkdown = "text/markdown"
)
func sluggify (s string) string {
	if s == "" {
		return s
	}
	return strings.Replace(s, "/", "-", -1)
}
func (c Content)IsSelf () bool {
	mimeComponents := strings.Split(c.MimeType, "/")
	return mimeComponents[0] == "text"
}
// handleMain serves /{year}/{month}/{day}/{hash} request
func (l *littr) handleContent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	date, err := time.Parse(time.RFC3339, fmt.Sprintf("%s-%s-%sT00:00:00+00:00", vars["year"], vars["month"], vars["day"]))
	if err != nil {
		l.handleError(w, r, err)
		return
	}
	hash := vars["hash"]

	db, err := orm.GetDB("default")
	if err != nil {
		l.handleError(w, r, err)
		return
	}

	sel := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "submitted_by", "handle", "content_items"."flags" from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where "submitted_at" > $1::date and "content_items"."key" ~* $2`
	rows, err := db.Query(sel, date, hash)
	if err != nil {
		l.handleError(w, r, err)
		return
	}
	m := contentModel{}
	for rows.Next() {
		p := Content{}
		err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Handle, &p.Flags)
		if err != nil {
			l.handleError(w, r, err)
			return
		}
		p.MimeTypeSlug = sluggify(p.MimeType)
		m.Title = p.Title
		m.Content = p
	}

	var terr error
	var t *template.Template
	t, terr = template.New("content.html").ParseFiles(templateDir + "content.html")
	if terr != nil {
		log.Print(terr)
	}
	t.Funcs(template.FuncMap{
		"formatDateInterval": relativeDate,
		"formatDate":         formatDate,
	})
	_, terr = t.New("link.html").ParseFiles(templateDir + "content/link.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("data.html").ParseFiles(templateDir + "content/data.html")
	if terr != nil {
		log.Print(terr)
	}

	terr = t.Execute(w, m)
	if terr != nil {
		log.Print(terr)
		return
	}
}