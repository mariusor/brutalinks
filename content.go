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
	"bytes"
)

type comment struct {
	Content
	Parent *comment
	Children []*comment
}

type contentModel struct {
	Title string
	Content comment
}
func (c contentModel)Level() int {
	return c.Content.Level()
}
func sluggify (s string) string {
	if s == "" {
		return s
	}
	return strings.Replace(s, "/", "-", -1)
}
func (c Content)ParentLink() string {
	if c.parentLink == "" {
		if c.Path == nil {
			c.parentLink = "/"
		} else {
			lastDotPos := bytes.LastIndex(c.Path, []byte(".")) + 1
			parentHash := c.Path[lastDotPos : lastDotPos+8]
			c.parentLink = fmt.Sprintf("/p/%s/%s", c.Hash(), parentHash)
		}
	}
	return c.parentLink
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
			"submitted_at", "submitted_by", "handle", "path", "content_items"."flags" from "content_items"
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
		err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Handle, &p.Path, &p.Flags)
		if err != nil {
			l.handleError(w, r, err)
			return
		}
		m.Title = string(p.Title)
		m.Content = comment {Content:p}
	}

	allComments := make([]*comment, 0)
	allComments = append(allComments, &m.Content)
	// comments
	selCom := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "submitted_by", "handle", "path", "content_items"."flags" from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where "path" <@ $1 order by "path" asc, "score" desc`
	{
		rows, err := db.Query(selCom, m.Content.Content.FullPath())

		if err != nil {
			l.handleError(w, r, err)
			return
		}
		for rows.Next() {
			c := Content{}
			err = rows.Scan(&c.Id, &c.Key, &c.MimeType, &c.Data, &c.Title, &c.Score, &c.SubmittedAt, &c.SubmittedBy, &c.Handle, &c.Path, &c.Flags)
			if err != nil {
				l.handleError(w, r, err)
				return
			}

			com := comment{Content: c}
			allComments = append(allComments, &com)
		}
	}

	for _, cur := range allComments {
		par := func (t []*comment, path []byte) *comment {
			// findParent
			if path == nil {
				return nil
			}
			for _, n := range t {
				if bytes.Equal(path, n.FullPath()) {
					return n
				}
			}
			return nil
		}(allComments, cur.Path)

		if par != nil {
			cur.Parent = par
			par.Children = append(par.Children, cur)
		}
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
		"sluggify": 		  sluggify,
	})
	_, terr = t.New("submit.html").ParseFiles(templateDir + "partials/content/submit.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("comments.html").ParseFiles(templateDir + "partials/content/comments.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("comment.html").ParseFiles(templateDir + "partials/content/comment.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("link.html").ParseFiles(templateDir + "partials/content/link.html")
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
		return
	}
}