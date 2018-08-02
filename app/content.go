package app

import (
	"bytes"
	"fmt"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
)

const Yay = "yay"
const Nay = "nay"

type comment struct {
	Item
	Parent   *comment
	Path     []byte
	FullPath []byte
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

func ReparentComments(allComments []*comment) {
	for _, cur := range allComments {
		par := func(t []*comment, path []byte) *comment {
			// findParent
			if len(path) == 0 {
				return nil
			}
			for _, n := range t {
				if bytes.Equal(path, n.FullPath) {
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
}

// ShowContent serves /{year}/{month}/{day}/{hash} request
// ShowContent serves /~{handle}/{hash} request
func ShowContent(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	items := make([]Item, 0)

	sel := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score",
			"submitted_at", "submitted_by", "handle", "path", "content_items"."flags" from "content_items"
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by"
			where "content_items"."key" ~* $1`
	rows, err := Db.Query(sel, hash)
	if err != nil {
		HandleError(w, r, StatusUnknown, err)
		return
	}
	m := contentModel{InvertedTheme: IsInverted(r)}
	p := models.Content{}
	var i Item
	for rows.Next() {
		var handle string
		err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &handle, &p.Path, &p.Flags)
		if err != nil {
			HandleError(w, r, StatusUnknown, err)
			return
		}
		m.Title = string(p.Title)
		i = LoadItem(p, handle)
		m.Content = comment{Item: i, Path: p.Path, FullPath: p.FullPath()}
	}
	if p.Data == nil {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("not found"))
		return
	}
	items = append(items, i)
	allComments := make([]*comment, 0)
	allComments = append(allComments, &m.Content)

	fullPath := bytes.Trim(m.Content.FullPath, " \n\r\t")
	if len(fullPath) > 0 {
		// comments
		selCom := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score", 
			"submitted_at", "submitted_by", "handle", "path", "content_items"."flags" from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where "path" <@ $1 and "path" is not null order by "path" asc, "score" desc`
		{
			rows, err := Db.Query(selCom, fullPath)

			if err != nil {
				HandleError(w, r, StatusUnknown, err)
				return
			}
			for rows.Next() {
				c := models.Content{}
				var handle string
				err = rows.Scan(&c.Id, &c.Key, &c.MimeType, &c.Data, &c.Title, &c.Score, &c.SubmittedAt, &c.SubmittedBy, &handle, &c.Path, &c.Flags)
				if err != nil {
					HandleError(w, r, StatusUnknown, err)
					return
				}

				i := LoadItem(c, handle)
				com := comment{Item: i, Path: c.Path, FullPath: c.FullPath()}
				items = append(items, i)
				allComments = append(allComments, &com)
			}
		}
	}

	ReparentComments(allComments)
	_, err = LoadVotes(CurrentAccount, items)
	if err != nil {
		log.Print(err)
		AddFlashMessage(fmt.Sprint(err), Error, r, w)
	}
	err = SessionStore.Save(r, w, GetSession(r))
	if err != nil {
		log.Print(err)
		AddFlashMessage(fmt.Sprint(err), Error, r, w)
	}
	if len(m.Title) > 0 {
		m.Title = fmt.Sprintf("%s", p.Title)
	} else {
		m.Title = fmt.Sprintf("%s comment", genitive(i.SubmittedBy))
	}
	RenderTemplate(r, w, "content", m)
}

func genitive(name string) string {
	l := len(name)
	if name[l-1:l] != "s" {
		return name + "'s"
	}
	return name + "'"
}

// HandleVoting serves /{year}/{month}/{day}/{hash}/{direction} request
// HandleVoting serves /~{handle}/{direction} request
func HandleVoting(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	items := make([]Item, 0)

	sel := `select "content_items"."id", "content_items"."key", "mime_type", "data", "title", "content_items"."score",
			"submitted_at", "submitted_by", "handle", "path", "content_items"."flags" from "content_items"
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by"
			where "content_items"."key" ~* $1`
	rows, err := Db.Query(sel, hash)
	if err != nil {
		HandleError(w, r, StatusUnknown, err)
		return
	}
	m := contentModel{InvertedTheme: IsInverted(r)}
	p := models.Content{}
	var i Item
	for rows.Next() {
		var handle string
		err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &handle, &p.Path, &p.Flags)
		if err != nil {
			HandleError(w, r, StatusUnknown, err)
			return
		}
		i = LoadItem(p, handle)
		m.Content = comment{Item: i, Path: p.Path, FullPath: p.FullPath()}
	}
	if p.Data == nil {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("not found"))
		return
	}
	items = append(items, i)

	multiplier := 0
	switch chi.URLParam(r, "direction") {
	case Yay:
		multiplier = 1
	case Nay:
		multiplier = -1
	}

	if CurrentAccount.IsLogged() {
		if _, err := AddVote(p, multiplier, CurrentAccount.Id); err != nil {
			log.Print(err)
		}
	} else {
		AddFlashMessage(fmt.Sprintf("unable to add vote as an %s user", anonymous), Error, r, w)
	}
	http.Redirect(w, r, i.PermaLink(), http.StatusFound)
}
