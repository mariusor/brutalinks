package main

import (
	"fmt"
	"github.com/astaxie/beego/orm"
	"github.com/gorilla/mux"
	"html/template"
	"log"
	"net/http"
	"time"
	"strings"
)

type Account struct {
	id        int64     `orm:id,"auto"`
	Key       string    `orm:key`
	Email     string    `orm:email`
	Handle    string    `orm:handle`
	Score     int64     `orm:score`
	CreatedAt time.Time `orm:created_at`
	UpdatedAt time.Time `orm:updated_at`
	flags     int8      `orm:flags`
	Metadata  []byte    `orm:metadata`
	Votes     map[int64]Vote
}

type userModel struct {
	Title string
	User  Account
	Items []Content
}
func (u *Account) VotedOn(i Content) *Vote{
	for _, v := range u.Votes {
		if v.id == i.id {
			return &v
		}
	}
	return nil
}
func (u *Account) LoadVotes(ids []int64) error {
	db, err := orm.GetDB("default")
	if err != nil {
		return err
	}
	// this here code following is the ugliest I wrote in quite a long time
	// so ugly it warrants its own fucking shame corner
	sids := make([]string, 0)
	for i := 0; i < len(ids); i++ {
		sids = append(sids, fmt.Sprintf("$%d", i+2))
	}
	iitems := make([]interface{}, len(ids)+1)
	iitems[0] = u.id
	for i, v := range ids {
		iitems[i+1] = v
	}
	sel := fmt.Sprintf(`select "id", "submitted_by", "submitted_at", "updated_at",
		"item_id", "weight", "flags"
	from "votes" where "submitted_by" = $1 and "item_id" in (%s)`,  strings.Join(sids, ", "))
	rows, err := db.Query(sel, iitems...)
	if err != nil {
		return err
	}
	for rows.Next() {
		v := Vote{}
		err = rows.Scan(&v.id, &v.submittedBy, &v.SubmittedAt, &v.UpdatedAt,
			&v.itemId, &v.weight, &u.flags)
		if err != nil {
			return err
		}
		u.Votes[v.id] = v
	}

	return nil
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

	u := Account{}
	selAcct := `select "id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags" from "accounts" where "handle" = $1`
	{
		rows, err := db.Query(selAcct, vars["handle"])
		if err != nil {
			l.handleError(w, r, err, -1)
			return
		}
		for rows.Next() {
			err = rows.Scan(&u.id, &u.Key, &u.Handle, &u.Email, &u.Score, &u.CreatedAt, &u.UpdatedAt, &u.Metadata, &u.flags)
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
		rows, err := db.Query(selC, u.id)
		if err != nil {
			l.handleError(w, r, err, -1)
			return
		}
		for rows.Next() {
			p := Content{}
			err = rows.Scan(&p.id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.flags, &p.Metadata)
			if err != nil {
				l.handleError(w, r, err, -1)
				return
			}
			p.Handle = u.Handle
			p.submittedBy = u.id
			m.Items = append(m.Items, p)
		}
	}
	err = CurrentAccount().LoadVotes(getAllIds(m.Items))
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
