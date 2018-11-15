package frontend

import (
	"fmt"
	"github.com/mariusor/littr.go/app/log"
	"net/http"
	"strings"

	"github.com/mariusor/littr.go/app"

	"github.com/go-chi/chi"
	"github.com/juju/errors"
)

const Yay = "yay"
const Nay = "nay"

type comments []*comment
type comment struct {
	app.Item
	Level    uint8
	Children comments
	Parent   *comment
}

type contentModel struct {
	Title         string
	InvertedTheme bool
	Content       comment
}

func loadComments(items []app.Item) comments {
	var comments = make([]*comment, len(items))
	for k, item := range items {
		com := comment{Item: item}
		comments[k] = &com
	}
	return comments
}

func (c comments) getItems() app.ItemCollection {
	var items = make(app.ItemCollection, len(c))
	for k, com := range c {
		items[k] = com.Item
	}
	return items
}

func (c comments) getItemsHashes() []string {
	var items = make([]string, len(c))
	for k, com := range c {
		items[k] = com.Item.Hash.String()
	}
	return items
}

func sluggify(s string) string {
	if s == "" {
		return s
	}
	return strings.Replace(s, "/", "-", -1)
}

func mimeTypeTagReplace(m string, t app.Tag) string {
	r := t.Name
	switch m {
	case "text/markdown":
		if t.Name[0] == '#' {
			r = fmt.Sprintf("[&#35;%s](%s)", t.Name[1:], t.URL)
		}
		if t.Name[0] == '@' || t.Name[0] == '~' {
			r = fmt.Sprintf("[&#126;%s](%s)", t.Name[1:], t.URL)
		}
	case "text/html":
		if t.Name[0] == '#' {
			r = fmt.Sprintf("<a href='%s'>&#35;%s</a>", t.URL, t.Name[1:])
		}
		if t.Name[0] == '@' || t.Name[0] == '~' {
			r = fmt.Sprintf("<a href='%s'>&#126;%s<a/>", t.URL, t.Name[1:])
		}
	}

	return r
}

func replaceTags(comments comments) {
	inRange := func (n string, nn []string) bool {
		for _, ts := range nn {
			if ts == n {
				return true
			}
		}
		return false
	}
	for _, cur := range comments {
		names := make([]string, 0)
		if cur.Metadata != nil && cur.Metadata.Tags != nil {
			for _, t := range cur.Metadata.Tags {
				if inRange(t.Name, names) {
					continue
				}
				r := mimeTypeTagReplace(cur.MimeType, t)
				cur.Data = strings.Replace(cur.Data, t.Name, r, -1)
				names = append(names, t.Name)
			}
			for _, t := range cur.Metadata.Mentions {
				if inRange(t.Name, names) {
					continue
				}
				r := mimeTypeTagReplace(cur.MimeType, t)
				cur.Data = strings.Replace(cur.Data, t.Name, r, -1)
			}
		}
	}
}

func addLevelComments(comments comments) {
	for _, cur := range comments {
		if len(cur.Children) > 0 {
			for _, child := range cur.Children {
				child.Level = cur.Level + 1
				addLevelComments(cur.Children)
			}
		}
	}
}

func reparentComments(allComments []*comment) {
	parFn := func(t []*comment, cur comment) *comment {
		for _, n := range t {
			if cur.Item.Parent != nil && cur.Item.Parent.Hash == n.Hash {
				return n
			}
		}
		return nil
	}

	for _, cur := range allComments {
		if par := parFn(allComments, *cur); par != nil {
			par.Children = append(par.Children, cur)
		}
	}
}

// ShowItem serves /{year}/{month}/{day}/{hash} request
// ShowItem serves /~{handle}/{hash} request
func ShowItem(w http.ResponseWriter, r *http.Request) {
	items := make([]app.Item, 0)
	ShowItemData = true

	m := contentModel{InvertedTheme: isInverted(r)}
	itemLoader, ok := app.ContextItemLoader(r.Context())
	if !ok {
		Logger.Error("could not load item repository from Context")
		return
	}
	handle := chi.URLParam(r, "handle")
	//acctLoader, ok := val.(app.CanLoadAccounts)
	//if !ok {
	//	Logger.Error("could not load account repository from Context")
	//}
	//act, err := acctLoader.LoadAccount(app.LoadAccountFilter{Handle: handle})

	hash := chi.URLParam(r, "hash")
	i, err := itemLoader.LoadItem(app.LoadItemsFilter{
		AttributedTo: []app.Hash{app.Hash(handle)},
		Key:          []string{hash},
	})
	if err != nil {
		Logger.Error(err.Error())
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	m.Content = comment{Item: i}
	if len(i.Data)+len(i.Title) == 0 {
		HandleError(w, r, http.StatusNotFound, errors.Errorf("not found"))
		return
	}
	items = append(items, i)
	allComments := make(comments, 1)
	allComments[0] = &m.Content

	contentItems, err := itemLoader.LoadItems(app.LoadItemsFilter{
		Context:  []string{m.Content.Hash.String()},
		MaxItems: MaxContentItems,
	})
	if err != nil {
		Logger.Error(err.Error())
		HandleError(w, r, http.StatusNotFound, err)
		return
	}
	allComments = append(allComments, loadComments(contentItems)...)

	replaceTags(allComments)
	reparentComments(allComments)
	addLevelComments(allComments)

	acc, ok := app.ContextCurrentAccount(r.Context())
	if ok && acc.IsLogged() {
		votesLoader, ok := app.ContextVoteLoader(r.Context())
		if ok {
			acc.Votes, err = votesLoader.LoadVotes(app.LoadVotesFilter{
				AttributedTo: []app.Hash{acc.Hash},
				ItemKey:      allComments.getItemsHashes(),
				MaxItems:     MaxContentItems,
			})
			if err != nil {
				Logger.Error(err.Error())
			}
		} else {
			Logger.Error("could not load vote repository from Context")
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
	if l == 0 {
		return name
	}
	if name[l-1:l] != "s" {
		return name + "'s"
	}
	return name + "'"
}

// HandleVoting serves /{year}/{month}/{day}/{hash}/{direction} request
// HandleVoting serves /~{handle}/{direction} request
func HandleVoting(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")

	val := r.Context().Value(app.RepositoryCtxtKey)
	itemLoader, ok := val.(app.CanLoadItems)
	if !ok {
		Logger.Error("could not load item repository from Context")
		return
	}

	p, err := itemLoader.LoadItem(app.LoadItemsFilter{Key: []string{hash}})
	if err != nil {
		Logger.Error(err.Error())
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
	url := ItemPermaLink(p)

	acc, ok := app.ContextCurrentAccount(r.Context())
	if acc.IsLogged() {
		if auth, ok := val.(app.Authenticated); ok {
			auth.WithAccount(acc)
		}
		voter, ok := val.(app.CanSaveVotes)
		backUrl := r.Header.Get("Referer")
		if !strings.Contains(backUrl, url) && strings.Contains(backUrl, app.Instance.BaseURL) {
			url = fmt.Sprintf("%s#item-%s", backUrl, p.Hash)
		}
		if !ok {
			Logger.Error("could not load vote repository from Context")
			return
		}
		v := app.Vote{
			SubmittedBy: acc,
			Item:        &p,
			Weight:      multiplier * app.ScoreMultiplier,
		}
		if _, err := voter.SaveVote(v); err != nil {
			Logger.WithContext(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
			}).Error(err.Error())
		}
	} else {
		addFlashMessage(Error, fmt.Sprintf("unable to add vote as an %s user", acc.Handle), r)
	}
	Redirect(w, r, url, http.StatusFound)
}
