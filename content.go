package main

import (
	"bytes"
	"fmt"
	"github.com/gorilla/mux"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"
	"math"
)

type comment struct {
	Content
	Parent   *comment
	Children []*comment
}

type contentModel struct {
	Title   string
	Content comment
}

func sluggify(s string) string {
	if s == "" {
		return s
	}
	return strings.Replace(s, "/", "-", -1)
}
func (c Content) ParentLink() string {
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
func (c Content) OPLink() string {
	if c.Path != nil {
		parentHash := c.Path[0 : 8]
		return fmt.Sprintf("/op/%s/%s", c.Hash(), parentHash)
	}
	return "/"
}
//func (c Content) ancestorLink(lvl int) string {
//
//}
func (c Content) IsSelf() bool {
	mimeComponents := strings.Split(c.MimeType, "/")
	return mimeComponents[0] == "text"
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
	items := make([]Content,0)

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
	m := contentModel{}
	p := Content{}
	for rows.Next() {
		err = rows.Scan(&p.id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.submittedBy, &p.Handle, &p.Path, &p.flags)
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
	items = append(items,p)

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
			_, err := app.Vote(p, multiplier, CurrentAccount().id)
			if err != nil {
				log.Print(err)
			}
			http.Redirect(w, r, p.PermaLink(), http.StatusFound)
		}
	}
	if r.Method == http.MethodPost {
		repl := Content{}

		repl.Data = []byte(r.PostFormValue("data"))
		if len(repl.Data) > 0 {
			now := time.Now()
			repl.MimeType = "text/plain"
			repl.submittedBy = CurrentAccount().id
			repl.SubmittedAt = now
			repl.UpdatedAt = now
			repl.Key = repl.GetKey()
			repl.Path = p.FullPath()
			log.Printf("generated key[%d] %s", len(repl.Key), repl.Key)

			ins := `insert into "content_items" ("key", "data", "mime_type", "submitted_by", "path", "submitted_at", "updated_at") values($1, $2, $3, $4, $5, $6, $7)`
			{
				res, err := db.Exec(ins, repl.Key, repl.Data, repl.MimeType, repl.submittedBy, repl.Path, repl.SubmittedAt, repl.UpdatedAt)
				if err != nil {
					log.Print(err)
				} else {
					if rows, _ := res.RowsAffected(); rows == 0 {
						log.Print(fmt.Errorf("could not save new reply %q", repl.Hash()))
					}
				}
			}
		}
		l.Vote(repl, 1, CurrentAccount().id)
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
			c := Content{}
			err = rows.Scan(&c.id, &c.Key, &c.MimeType, &c.Data, &c.Title, &c.Score, &c.SubmittedAt, &c.submittedBy, &c.Handle, &c.Path, &c.flags)
			if err != nil {
				l.handleError(w, r, err, -1)
				return
			}

			com := comment{Content: c}
			items = append(items,c)
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
	err = CurrentAccount().LoadVotes(getAllIds(items))
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
		"title":			  func(t []byte) string { return string(t) },
		"mod":			  	  func(lvl int) float64 { return math.Mod(float64(lvl), float64(10)) },
		"getProviders": 	  getAuthProviders,
		"CurrentAccount": 	  CurrentAccount,
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
