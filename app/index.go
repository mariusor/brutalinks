package main

import (
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"time"
	"models"
)

const (
	MaxContentItems = 200
)

type indexModel struct {
	Title string
	Items []models.Content
}

func relativeDate(c time.Time) string {
	i := time.Now().Sub(c)
	pluralize := func(d float64, unit string) string {
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
			val = math.Mod(seconds, 60)
			unit = "second"
		} else {
			val = math.Mod(minutes, 60)
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
	} else if hours < 876000 {
		val = hours / 87600
		unit = "decade"
	} else {
		val = hours / 876000
		unit = "century"
	}
	return fmt.Sprintf("%.0f %s %s", val, pluralize(val, unit), when)
}
func formatDate(c time.Time) string {
	return c.Format("2006-01-02T15:04:05.000-07:00")
}

func getAuthProviders() map[string]string {
	p := make(map[string]string)
	p["github"] = "Github"
	//p["gitlab"] = "Gitlab"
	//p["google"] = "Google"
	//p["facebook"] = "Facebook"

	return p
}

// handleMain serves / request
func (l *littr) handleIndex(w http.ResponseWriter, r *http.Request) {
	m := indexModel{Title: "Index"}

	db := l.Db

	sel := fmt.Sprintf(`select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "submitted_by", "handle", "content_items"."flags" 
		from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
		where path is NULL
	order by "score" desc, "submitted_at" desc limit %d`, MaxContentItems)
	rows, err := db.Query(sel)
	if err != nil {
		l.handleError(w, r, err, -1)
		return
	}
	for rows.Next() {
		p := models.Content{}
		err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Handle, &p.Flags)
		if err != nil {
			l.handleError(w, r, err, -1)
			return
		}
		m.Items = append(m.Items, p)
	}

	err = l.LoadVotes(CurrentAccount(), getAllIds(m.Items))
	if err != nil {
		log.Print(err)
	}

	err = l.session.Save(r, w, l.Session(r))
	if err != nil {
		log.Print(err)
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
		"sluggify":           sluggify,
		"title":			  func(t []byte) string { return string(t) },
		"getProviders": 	  getAuthProviders,
		"CurrentAccount": 	  CurrentAccount,
		"LoadFlashMessages":  LoadFlashMessages,
		"CleanFlashMessages":  CleanFlashMessages,
	})
	_, terr = t.New("items.html").ParseFiles(templateDir + "partials/content/items.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("link.html").ParseFiles(templateDir + "partials/content/link.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("flash.html").ParseFiles(templateDir + "partials/flash.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("meta.html").ParseFiles(templateDir + "partials/content/meta.html")
	if terr != nil {
		log.Print(terr)
	}
	_, terr = t.New("data.html").ParseFiles(templateDir + "partials/content/data.html")
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
