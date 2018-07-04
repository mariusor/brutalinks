package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"math"
	"models"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type comment struct {
	models.Content
	Parent   *comment
	Children []*comment
}

type contentModel struct {
	Title         string
	InvertedTheme bool
	Content       comment
}

func sluggify(s string) string {
	if s == "" {
		return s
	}
	return strings.Replace(s, "/", "-", -1)
}

// handleMain serves /{year}/{month}/{day}/{hash} request
func (l *littr) handleContent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	date, err := time.Parse(time.RFC3339, fmt.Sprintf("%s-%s-%sT00:00:00+00:00", vars["year"], vars["month"], vars["day"]))
	if err != nil {
		l.handleError(w, r, err, -1)
		return
	}
	hash := vars["hash"]
	items := make([]models.Content, 0)

	db := l.Db

	sel := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score",
			"submitted_at", "submitted_by", "handle", "path", "content_items"."flags" from "content_items"
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by"
			where "submitted_at" > $1::date and "content_items"."key" ~* $2`
	rows, err := db.Query(sel, date, hash)
	if err != nil {
		l.handleError(w, r, err, -1)
		return
	}
	m := contentModel{InvertedTheme: l.InvertedTheme}
	p := models.Content{}
	for rows.Next() {
		err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Handle, &p.Path, &p.Flags)
		if err != nil {
			l.handleError(w, r, err, -1)
			return
		}
		m.Title = string(p.Title)
		m.Content = comment{Content: p}
	}
	if p.Data == nil {
		l.handleError(w, r, fmt.Errorf("not found"), http.StatusNotFound)
		return
	}
	items = append(items, p)

	if r.Method == http.MethodGet {
		q := r.URL.Query()
		yay := len(q["yay"]) > 0
		nay := len(q["nay"]) > 0
		multiplier := 0

		if yay || nay {
			if nay {
				multiplier = -1
			}
			if yay {
				multiplier = 1
			}
			_, err := app.Vote(p, multiplier, CurrentAccount().Id)
			if err != nil {
				log.Print(err)
			}
			http.Redirect(w, r, p.PermaLink(), http.StatusFound)
		}
	}
	if r.Method == http.MethodPost {
		repl := models.Content{}

		repl.Data = []byte(r.PostFormValue("data"))
		if len(repl.Data) > 0 {
			now := time.Now()
			repl.MimeType = "text/plain"
			repl.SubmittedBy = CurrentAccount().Id
			repl.SubmittedAt = now
			repl.UpdatedAt = now
			repl.Key = repl.GetKey()
			repl.Path = p.FullPath()
			log.Printf("generated key[%d] %s", len(repl.Key), repl.Key)

			ins := `insert into "content_items" ("key", "data", "mime_type", "submitted_by", "path", "submitted_at", "updated_at") values($1, $2, $3, $4, $5, $6, $7)`
			{
				res, err := db.Exec(ins, repl.Key, repl.Data, repl.MimeType, repl.SubmittedBy, repl.Path, repl.SubmittedAt, repl.UpdatedAt)
				if err != nil {
					log.Print(err)
				} else {
					if rows, _ := res.RowsAffected(); rows == 0 {
						log.Print(fmt.Errorf("could not save new reply %q", repl.Hash()))
					}
				}
			}
		}
		l.Vote(repl, 1, CurrentAccount().Id)
		http.Redirect(w, r, p.PermaLink(), http.StatusFound)
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
			l.handleError(w, r, err, -1)
			return
		}
		for rows.Next() {
			c := models.Content{}
			err = rows.Scan(&c.Id, &c.Key, &c.MimeType, &c.Data, &c.Title, &c.Score, &c.SubmittedAt, &c.SubmittedBy, &c.Handle, &c.Path, &c.Flags)
			if err != nil {
				l.handleError(w, r, err, -1)
				return
			}

			com := comment{Content: c}
			items = append(items, c)
			allComments = append(allComments, &com)

		}
	}

	for _, cur := range allComments {
		par := func(t []*comment, path []byte) *comment {
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
	err = l.LoadVotes(CurrentAccount(), getAllIds(items))
	if err != nil {
		log.Print(err)
	}
	err = l.session.Save(r, w, l.Session(r))
	if err != nil {
		log.Print(err)
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
		"sluggify":           sluggify,
		"title":              func(t []byte) string { return string(t) },
		"mod":                func(lvl int) float64 { return math.Mod(float64(lvl), float64(10)) },
		"getProviders":       getAuthProviders,
		"CurrentAccount":     CurrentAccount,
		"LoadFlashMessages":  LoadFlashMessages,
		"CleanFlashMessages": CleanFlashMessages,
	})
	_, terr = t.New("submit.html").ParseFiles(templateDir + "partials/content/submit.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("flash.html").ParseFiles(templateDir + "partials/flash.html")
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
	_, terr = t.New("meta.html").ParseFiles(templateDir + "partials/content/meta.html")
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
