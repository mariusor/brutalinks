package main

import (
	"time"
	"net/http"
	"github.com/astaxie/beego/orm"
	"fmt"
	"html/template"
	"math"
)

const (
	FLAGS_NONE = 0
	FLAGS_DELETED = 1
	MimeTypeURL = "application/url"
	ScoreMultiplier = 10000.0
	ScoreMaxK = 10000.0
	ScoreMaxM = 10000000.0
	ScoreMaxB = 10000000000.0
)

type Content struct {
	Id 	int32	`orm:id,"auto"`
	Key string	`orm:key,size(56)`
	Title string	`orm:title`
	MimeType string	`orm:mime_type`
	Data []byte `orm:data`
	Score int64 `orm:score`
	CreatedAt time.Time `orm:created_at`
	Flags int8 `NewRouterorm:flags`
	PermaLink string
}

type indexModel struct {
	Title string
	Auth map[string]string
	Content []Content
}
func (c Content)Deleted () bool {
	return c.Flags & FLAGS_DELETED == FLAGS_DELETED
}

func (c Content)ScoreFmt () string {
	score := 0.0
	units := ""
	base := float64(c.Score) / ScoreMultiplier
	d := math.Ceil(math.Log10(base))
	if d < 5 {
		score = math.Ceil(base)
		return fmt.Sprintf("%d", int(score))
	} else if d < 8 {
		score = base / ScoreMaxK
		units = "K"
	} else if d < 11 {
		score = base / ScoreMaxM
		units = "M"
	} else if d < 13 {
		score = base / ScoreMaxB
		units = "B"
	} else {
		sign := ""
		if base < 0 {
			sign = "-"
		}
		return fmt.Sprintf("%s%s", sign, "∞")
	}

	return fmt.Sprintf("%3.1f%s", score, units)
}

/*
func (c Content)Output () string {
	if c.Flags & FLAGS_DELETED == FLAGS_DELETED {
		return fmt.Sprintf(`<tt data-id="%d" style="font-decoration:strike-through">deleted</tt>`, c.Id)
	}
	if c.MimeType == MimeTypeURL {
		return fmt.Sprintf(`<a data-id="%d" datetime="%s" href="%s">%s</a>`, c.Id, c.CreatedAt.Format("2009-01-01 21:11:22.222 GMT"), c.Data, c.Title)
	}
	return `<em>unknown</em>`
}
*/

// handleMain serves / request
func (l *littr) handleIndex(w http.ResponseWriter, r *http.Request) {
	m := indexModel{Title: "Index"}
	m.Auth = make(map[string]string)
	m.Auth["github"] = "Github"

	db, err := orm.GetDB("default")
	if err != nil {
		l.handleError(w, r, err)
		return
	}

	rows, err := db.Query("select id, key, mime_type, data, title, score, created_at, flags from content order by score desc")
	if err != nil {
		l.handleError(w, r, err)
		return
	}
	for rows.Next() {
		p := Content{}
		err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.CreatedAt, &p.Flags)
		if err != nil {
			l.handleError(w, r, err)
			return
		}
		p.PermaLink = fmt.Sprintf("http://%s:3000/%4d/%02d/%02d/%s", listenHost, p.CreatedAt.Year(),  p.CreatedAt.Month(), p.CreatedAt.Day(), p.Key[0:8])

		m.Content = append(m.Content, p)
	}

	t, _ := template.New("index.html").ParseFiles(templateDir + "index.html")
//	t.New("head.html").ParseFiles(templateDir + "content/head.html")
	t.New("deleted.html").ParseFiles(templateDir + "content/deleted.html")
	t.New("link.html").ParseFiles(templateDir + "content/link.html")

	t.Execute(w, m)
}