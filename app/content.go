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

type comments []*comment
type comment struct {
	models.Item
	Parent   *comment
	Path     []byte
	FullPath []byte
	Children comments
}

type contentModel struct {
	Title         string
	InvertedTheme bool
	Content       comment
}

func loadComments(items []models.Item) comments {
	var comments = make([]*comment, len(items))
	for k, item := range items {
		//l := LoadItem(item)
		com := comment{Item: item, Path: item.Path, FullPath: item.FullPath}

		comments[k] = &com
	}
	return comments
}
func (c comments) getItems() []models.Item {
	var items = make([]models.Item, len(c))
	for k, com := range c {
		items[k] = com.Item
	}
	return items
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

// ShowItem serves /{year}/{month}/{day}/{hash} request
// ShowItem serves /~{handle}/{hash} request
func ShowItem(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	items := make([]models.Item, 0)
	ShowItemData = true

	m := contentModel{InvertedTheme: isInverted(r)}

	i, err := models.LoadItemByHash(hash)
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	m.Content = comment{Item: i, Path: i.Path, FullPath: i.FullPath}
	if i.Data == "" {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("not found"))
		return
	}
	items = append(items, i)
	allComments := make(comments, 1)
	allComments[0] = &m.Content

	fullPath := bytes.Trim(m.Content.FullPath, " \n\r\t")
	contentItems, err := models.LoadItemsByPath(fullPath, MaxContentItems)
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	allComments = append(allComments, loadComments(contentItems)...)

	ReparentComments(allComments)
	_, err = LoadVotes(CurrentAccount, items)
	if err != nil {
		log.Print(err)
		AddFlashMessage(Error,fmt.Sprint(err), r, w)
	}
	err = SessionStore.Save(r, w, GetSession(r))
	if err != nil {
		log.Print(err)
		AddFlashMessage(Error, fmt.Sprint(err), r, w)
	}
	if len(m.Title) > 0 {
		m.Title = fmt.Sprintf("%s", i.Title)
	} else {
		// FIXME(marius): we lost the handle of the account
		m.Title = fmt.Sprintf("%s comment", genitive(m.Content.SubmittedBy.Handle))
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
	p, err := models.LoadItem( hash)
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}

	multiplier := 0
	switch chi.URLParam(r, "direction") {
	case Yay:
		multiplier = 1
	case Nay:
		multiplier = -1
	}

	if CurrentAccount.IsLogged() {
		if _, err := AddVote(p, multiplier, CurrentAccount.Hash); err != nil {
			log.Print(err)
		}
	} else {
		AddFlashMessage(Error, fmt.Sprintf("unable to add vote as an %s user", anonymous), r, w)
	}
	Redirect(w, r, permaLink(p), http.StatusFound)
}
