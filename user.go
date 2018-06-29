package main

import (
	"fmt"
	"github.com/astaxie/beego/orm"
	"github.com/gorilla/mux"
	"html/template"
	"log"
	"net/http"
	"time"
)

type User struct {
	Id        int64     `orm:id,"auto"`
	Key       string    `orm:key`
	Email     string    `orm:email`
	Handle    string    `orm:handle`
	Score     int64     `orm:score`
	CreatedAt time.Time `orm:created_at`
	UpdatedAt time.Time `orm:updated_at`
	Flags     int8      `orm:flags`
	Metadata  []byte    `orm:metadata`
}

type userModel struct {
	Title string
	User  User
	Items []Content
}

// handleMain serves /~{user}
func (l *littr) handleUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	db, err := orm.GetDB("default")
	if err != nil {
		l.handleError(w, r, err, -1)
		return
	}
	m := userModel{}

	u := User{}
	selAcct := `select "id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags" from "accounts" where "handle" = $1`
	{
		rows, err := db.Query(selAcct, vars["handle"])
		if err != nil {
			l.handleError(w, r, err, -1)
			return
		}
		for rows.Next() {
			err = rows.Scan(&u.Id, &u.Key, &u.Handle, &u.Email, &u.Score, &u.CreatedAt, &u.UpdatedAt, &u.Metadata, &u.Flags)
			if err != nil {
				l.handleError(w, r, err, -1)
				return
			}
		}

		m.Title = fmt.Sprintf("Activity %s", u.Handle)
		m.User = u
	}
	selC := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "content_items"."flags", "content_items"."metadata"  from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where "submitted_by" = $1 order by "submitted_at" desc`
	{
		rows, err := db.Query(selC, u.Id)
		if err != nil {
			l.handleError(w, r, err, -1)
			return
		}
		for rows.Next() {
			p := Content{}
			err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.Flags, &p.Metadata)
			if err != nil {
				l.handleError(w, r, err, -1)
				return
			}
			p.Handle = u.Handle
			p.SubmittedBy = u.Id
			m.Items = append(m.Items, p)
		}
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
		"title":			  func(t []byte) string { return string(t) },
	})
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("items.html").ParseFiles(templateDir + "partials/content/items.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("score.html").ParseFiles(templateDir + "partials/content/score.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("link.html").ParseFiles(templateDir + "partials/content/link.html")
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
	terr = t.Execute(w, m)
	if terr != nil {
		log.Print(terr)
	}
}
