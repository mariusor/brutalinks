package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"html/template"
	"log"
	"net/http"
	"models"
)

type userModel struct {
	Title string
	User  models.Account
	Items []models.Content
}

// handleMain serves /~{user}
func (l *littr) handleUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	db := l.Db
	m := userModel{}

	u := models.Account{}
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
			"submitted_at", "content_items"."flags", "content_items"."metadata", "accounts"."handle" f from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where "submitted_by" = $1 order by "submitted_at" desc`
	{
		rows, err := db.Query(selC, u.Id)
		if err != nil {
			l.handleError(w, r, err, -1)
			return
		}
		for rows.Next() {
			p := models.Content{}
			err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.Flags, &p.Metadata, &p.Handle)
			if err != nil {
				l.handleError(w, r, err, -1)
				return
			}
			//p.Handle = u.Handle
			p.SubmittedBy = u.Id
			m.Items = append(m.Items, p)
		}
	}
	err := l.LoadVotes(CurrentAccount(), getAllIds(m.Items))
	if err != nil {
		log.Print(err)
	}

	err = l.session.Save(r, w, l.Session(r))
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
