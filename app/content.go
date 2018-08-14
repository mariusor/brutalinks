package app

import (
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
		com := comment{Item: item, /*Path: item.Path, FullPath: item.FullPath*/}

		comments[k] = &com
	}
	return comments
}

func (c comments) getItems() models.ItemCollection {
	var items = make(models.ItemCollection, len(c))
	for k, com := range c {
		items[k] = com.Item
	}
	return items
}

func (c comments) getItemsHashes() []string {
	var items = make([]string, len(c))
	for k, com := range c {
		items[k] = com.Item.Hash
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
		par := func(t []*comment, cur comment) *comment {
			for _, n := range t {
				if cur.Parent != nil && cur.Parent.Hash == n.Hash {
					return n
				}
			}
			return nil
		}(allComments, *cur)

		if par != nil {
			par.Children = append(par.Children, cur)
		}
	}
}

// ShowItem serves /{year}/{month}/{day}/{hash} request
// ShowItem serves /~{handle}/{hash} request
func ShowItem(w http.ResponseWriter, r *http.Request) {
	items := make([]models.Item, 0)
	ShowItemData = true

	m := contentModel{InvertedTheme: isInverted(r)}
	val := r.Context().Value(ServiceCtxtKey)
	itemLoader, ok := val.(models.CanLoadItems)
	if ok {
		log.Infof("loaded LoaderService of type %T", itemLoader)
	} else {
		log.Errorf("could not load item loader service from Context")
		return
	}
	//handle := chi.URLParam(r, "handle")
	//acctLoader, ok := val.(models.CanLoadAccounts)
	//if ok {
	//	log.Infof("loaded LoaderService of type %T", acctLoader)
	//} else {
	//	log.Errorf("could not load account loader service from Context")
	//}
	//act, err := acctLoader.LoadAccount(models.LoadAccountFilter{Handle: handle})

	hash := chi.URLParam(r, "hash")
	i, err  := itemLoader.LoadItem(models.LoadItemsFilter{
		//SubmittedBy: []string{act.Hash},
		Key: []string{hash},
	})
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	m.Content = comment{Item: i, /*Path: i.Path, FullPath: i.FullPath*/}
	if i.Data == "" {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("not found"))
		return
	}
	items = append(items, i)
	allComments := make(comments, 1)
	allComments[0] = &m.Content

	contentItems, err := itemLoader.LoadItems(models.LoadItemsFilter{
		InReplyTo: []string{m.Content.Hash},
		MaxItems:  MaxContentItems,
	})
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	allComments = append(allComments, loadComments(contentItems)...)

	ReparentComments(allComments)

	if CurrentAccount.IsLogged() {
		votesLoader, ok := val.(models.CanLoadVotes)
		if ok {
			log.Infof("loaded LoaderService of type %T", itemLoader)
			CurrentAccount.Votes, err = votesLoader.LoadVotes(models.LoadVotesFilter{
				SubmittedBy: []string{CurrentAccount.Hash,},
				ItemKey:     allComments.getItemsHashes(),
				MaxItems:    MaxContentItems,
			})
			if err != nil {
				log.Error(err)
			}
		} else {
			log.Errorf("could not load vote loader service from Context")
		}
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
	p, err  := models.Service.LoadItem(models.LoadItemsFilter{Key: []string{hash}})
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
