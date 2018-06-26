package main

import (
	"net/http"
	"html/template"
	"github.com/gorilla/mux"
	"github.com/astaxie/beego/orm"
	"fmt"
	"time"
)

type contentModel struct {
	Title string
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

	rows, err := db.Query("select id, key, mime_type, data, title, score, created_at, flags from content where created_at > $1::date and key ~* $2", date, hash)
	if err != nil {
		l.handleError(w, r, err)
		return
	}
	c := contentModel{}
	for rows.Next() {
		p := Content{}
		err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.CreatedAt, &p.Flags)
		if err != nil {
			l.handleError(w, r, err)
			return
		}
		c.Title = p.Title
	}

	t, _ := template.New("content.html").ParseFiles(templateDir + "content.html")
	t.New("head.html").ParseFiles(templateDir + "content/head.html")
	t.Execute(w, c)
}