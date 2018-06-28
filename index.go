package main

import (
	"time"
	"fmt"
	"bytes"
	"html/template"
	"math"
	"log"
	"strings"
	"net/http"
	"github.com/astaxie/beego/orm"
)

const (
	FlagsNone       = 0
	FlagsDeleted    = 1
	MimeTypeURL     = "application/url"
	ScoreMultiplier = 10000.0
	ScoreMaxK       = 10000.0
	ScoreMaxM       = 10000000.0
	ScoreMaxB       = 10000000000.0
	MaxContentItems = 200
)

type Content struct {
	Id           int64     `orm:id,"auto"`
	Key          []byte    `orm:key,size(56)`
	Title        string    `orm:title`
	MimeType     string    `orm:mime_type`
	Data         []byte    `orm:data`
	Score        int64     `orm:score`
	SubmittedAt  time.Time `orm:created_at`
	SubmittedBy  int64     `orm:submitted_by`
	UpdatedAt    time.Time `orm:updated_at`
	Handle       string    `orm:handle`
	Flags        int8      `orm:flags`
	Metadata     []byte    `orm:metadata`
	Path     	 []byte    `orm:path`
	fullPath	 []byte
	parentLink   string
}

type indexModel struct {
	Title string
	Auth  map[string]string
	Items []Content
}
func (c Content)IsTop() bool {
	return c.Path == nil
}
func (c Content)Hash() string {
	return c.Hash8()
}
func (c Content)Hash8() string {
	return string(c.Key[0:8])
}
func (c Content)Hash16() string {
	return string(c.Key[0:16])
}
func (c Content)Hash32() string {
	return string(c.Key[0:32])
}
func (c Content)Hash64() string {
	return string(c.Key)
}
func (c Content)PermaLink() string {
	if c.SubmittedAt.IsZero() {
		return ""
	}
	return fmt.Sprintf("/%4d/%02d/%02d/%s", c.SubmittedAt.Year(),  c.SubmittedAt.Month(), c.SubmittedAt.Day(), c.Key[0:8])
}
func (c *Content)FullPath() []byte {
	if c.fullPath == nil {
		c.fullPath = append(c.fullPath, c.Path...)
		if len(c.fullPath) > 0 {
			c.fullPath = append(c.fullPath, byte('.'))
		}
		c.fullPath = append(c.fullPath, c.Key...)
	}
	return c.fullPath
}
func (c Content)Level() int {
	if c.Path == nil {
		return 0
	}
	return bytes.Count(c.FullPath(), []byte("."))
}
func (c Content)Deleted () bool {
	return c.Flags &FlagsDeleted == FlagsDeleted
}
func (c Content)IsLink () bool {
	return c.MimeType == MimeTypeURL
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
func (c Content)GetDomain() string {
	if ! c.IsLink() {
		return ""
	}
	return strings.Split(string(c.Data), "/")[2]
}
func relativeDate (c time.Time) string {
	i := time.Now().Sub(c)
	pluralize := func (d float64, unit string) string {
		if math.Round(d) != 1 {
			if unit == "century" {
				unit = "centurie"
			}
			return unit + "s"
		}
		return unit
	}
	val := 0.0
	unit := ""
	when := "ago"

	hours := math.Abs(i.Hours())
	minutes := math.Abs(i.Minutes())
	seconds := math.Abs(i.Seconds())

	if i.Seconds() < 0 {
		// we're in the future
		when = "in the future"
	}
	if seconds < 30 {
		return "now"
	}
	if hours < 1 {
		if minutes < 1 {
			val = math.Mod(seconds,60)
			unit = "second"
		} else {
			val = math.Mod(minutes,60)
			unit = "minute"
		}
	} else if hours < 24 {
		val = hours
		unit = "hour"
	} else if hours < 168 {
		val = hours / 24
		unit = "day"
	} else if hours < 672 {
		val = hours / 168
		unit = "week"
	} else if hours < 8760 {
		val = hours / 672
		unit = "month"
	} else if hours < 87600 {
		val = hours / 8760
		unit = "year"
	}  else if hours < 876000 {
		val = hours / 87600
		unit = "decade"
	} else {
		val = hours / 876000
		unit = "century"
	}
	return fmt.Sprintf("%.0f %s %s", val, pluralize(val, unit), when)
}
func formatDate (c time.Time) string {
	return c.Format("2006-01-02T15:04:05.000-07:00")
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

	sel := fmt.Sprintf(`select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "submitted_by", "handle", "content_items"."flags" 
		from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
		where path is NULL
	order by "score" desc, "submitted_at" desc limit %d`, MaxContentItems)
	rows, err := db.Query(sel)
	if err != nil {
		l.handleError(w, r, err)
		return
	}
	for rows.Next() {
		p := Content{}
		err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Handle, &p.Flags)
		if err != nil {
			l.handleError(w, r, err)
			return
		}
		m.Items = append(m.Items, p)
	}

	var terr error
	var t *template.Template
	t, terr = template.New("index.html").ParseFiles(templateDir + "index.html")
	if terr != nil {
		log.Print(terr)
	}
	t.Funcs(template.FuncMap{
		"formatDateInterval": relativeDate,
		"formatDate":         formatDate,
	})
	_, terr = t.New("items.html").ParseFiles(templateDir + "partials/content/items.html")
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