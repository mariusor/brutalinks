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
	MaxContentItems = 200
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
func relativeDate (c time.Time) string {
	i := time.Now().Sub(c)
	pluralize := func (d float64) string {
		if d != 1 {
			return "s"
		}
		return ""
	}
	unit := ""
	val := 0.0

	if i.Hours() > 1 {
		val = math.Mod(i.Hours(),24)
		unit = "hour"
	} else if i.Hours() > 24 {
		val = i.Hours()/24
		unit = "day"
	} else if i.Hours() > 168 {
		val = i.Hours()/168
		unit = "week"
	} else if i.Hours() > 672 {
		val = i.Hours()/672
		unit = "month"
	} else {
		if i.Seconds() > 0 {
			val = math.Mod(i.Seconds(),60)
			unit = "second"
		}
		if i.Minutes() > 0 {
			val = math.Mod(i.Minutes(),60)
			unit = "minute"
		}
	}
	return fmt.Sprintf("%.0f %s%s ago", val, unit, pluralize(val))
}
func formatDate (c time.Time) string {
	return c.Format("2006-01-02T15:04:05.000-07:00")
}
func (c Content)ScoreFmt () string {
	score := 0.0
	units := ""
	base := float64(c.Score) / ScoreMultiplier
	d := math.Ceil(math.Log10(math.Abs(base)))
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

	rows, err := db.Query("select id, key, mime_type, data, title, score, created_at, flags from content order by score desc limit 200")
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
	t.Funcs(template.FuncMap{
		"formatDateInterval": relativeDate,
		"formatDate":         formatDate,
	})
//	t.New("head.html").ParseFiles(templateDir + "content/head.html")
	t.New("deleted.html").ParseFiles(templateDir + "content/deleted.html")
	t.New("link.html").ParseFiles(templateDir + "content/link.html")

	t.Execute(w, m)
}