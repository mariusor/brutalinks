package frontend

import (
	"fmt"
	"github.com/mariusor/littr.go/app/log"
	"net/http"
	"path"
	"strings"

	"github.com/mariusor/littr.go/app"

	"github.com/go-chi/chi"
	"github.com/juju/errors"
)

const Edit = "edit"
const Delete = "rm"
const Report = "bad"
const Yay = "yay"
const Nay = "nay"

type comments []*comment
type comment struct {
	app.Item
	Level    uint8
	Edit     bool
	Children comments
	Parent   *comment
}

type contentModel struct {
	Title   string
	Content comment
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
	var cls string

	if t.Name[0] == '#' {
		cls = "tag"
	}
	if t.Name[0] == '@' || t.Name[0] == '~' {
		cls = "mention"
	}

	return fmt.Sprintf("<a href='%s' class='%s'>%s</a>", t.URL, cls, t.Name[1:])
}

func inRange(n string, nn []string) bool {
	for _, ts := range nn {
		if ts == n {
			return true
		}
	}
	return false
}

func replaceTagsInItem(cur app.Item) string {
	dat := cur.Data
	if cur.Metadata == nil {
		return dat
	}
	names := make([]string, 0)
	if cur.Metadata.Tags != nil {
		for _, t := range cur.Metadata.Tags {
			if inRange(t.Name, names) {
				continue
			}
			r := mimeTypeTagReplace(cur.MimeType, t)
			dat = strings.Replace(dat, t.Name, r, -1)
			names = append(names, t.Name)
		}
	}
	if cur.Metadata.Mentions != nil {
		for _, t := range cur.Metadata.Mentions {
			if inRange(t.Name, names) {
				continue
			}
			r := mimeTypeTagReplace(cur.MimeType, t)
			dat = strings.Replace(dat, t.Name, r, -1)
		}
	}
	return dat
}

func replaceTags(comments comments) {
	for _, cur := range comments {
		cur.Data = replaceTagsInItem(cur.Item)
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

// ShowItem serves /~{handle}/{hash} request
// ShowItem serves /~{handle}/{hash}/edit request
// ShowItem serves /{year}/{month}/{day}/{hash} request
// ShowItem serves /{year}/{month}/{day}/{hash}/edit request
func (h *handler) ShowItem(w http.ResponseWriter, r *http.Request) {
	items := make([]app.Item, 0)

	m := contentModel{}
	itemLoader, ok := app.ContextItemLoader(r.Context())
	if !ok {
		h.logger.Error("could not load item repository from Context")
		return
	}
	handle := chi.URLParam(r, "handle")
	hash := chi.URLParam(r, "hash")

	i, err := itemLoader.LoadItem(app.LoadItemsFilter{
		AttributedTo: []app.Hash{app.Hash(handle)},
		Key:          []string{hash},
	})
	if err != nil {
		h.logger.Error(err.Error())
		h.HandleError(w, r, errors.NewNotFound(err, "Item"))
		return
	}
	m.Content = comment{Item: i}
	if !i.Deleted() && len(i.Data)+len(i.Title) == 0 {
		h.HandleError(w, r, errors.NotFoundf("Item"))
		return
	}
	url := r.URL
	maybeEdit := path.Base(url.Path)

	if maybeEdit != hash && maybeEdit == Edit {
		if !sameHash(m.Content.SubmittedBy.Hash, h.account.Hash) {
			url.Path = path.Dir(url.Path)
			h.Redirect(w, r, url.RequestURI(), http.StatusFound)
			return
		}
		m.Content.Edit = true
	}

	items = append(items, i)
	allComments := make(comments, 1)
	allComments[0] = &m.Content

	contentItems, err := itemLoader.LoadItems(app.LoadItemsFilter{
		Context:  []string{m.Content.Hash.String()},
		MaxItems: MaxContentItems,
	})
	if err != nil {
		h.logger.Error(err.Error())
		h.HandleError(w, r, errors.NewNotFound(err, "not found"))
		return
	}
	allComments = append(allComments, loadComments(contentItems)...)

	//replaceTags(allComments)
	reparentComments(allComments)
	addLevelComments(allComments)

	if ok && h.account.IsLogged() {
		votesLoader, ok := app.ContextVoteLoader(r.Context())
		if ok {
			h.account.Votes, err = votesLoader.LoadVotes(app.LoadVotesFilter{
				AttributedTo: []app.Hash{h.account.Hash},
				ItemKey:      allComments.getItemsHashes(),
				MaxItems:     MaxContentItems,
			})
			if err != nil {
				h.logger.Error(err.Error())
			}
		} else {
			h.logger.Error("could not load vote repository from Context")
		}
	}
	if len(m.Title) > 0 {
		m.Title = fmt.Sprintf("%s", i.Title)
	} else {
		// FIXME(marius): we lost the handler of the account
		m.Title = fmt.Sprintf("%s comment", genitive(m.Content.SubmittedBy.Handle))
	}
	h.RenderTemplate(r, w, "content", m)
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

// HandleDelete serves /{year}/{month}/{day}/{hash}/Delete POST request
// HandleDelete serves /~{handle}/Delete request
func (h *handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")

	val := r.Context().Value(app.RepositoryCtxtKey)
	itemLoader, ok := val.(app.CanLoadItems)
	if !ok {
		h.logger.Error("could not load item repository from Context")
		return
	}
	p, err := itemLoader.LoadItem(app.LoadItemsFilter{Key: []string{hash}})
	if err != nil {
		h.logger.Error(err.Error())
		h.HandleError(w, r, errors.NewNotFound(err, "not found"))
		return
	}

	url := ItemPermaLink(p)
	backUrl := r.Header.Get("Referer")
	if !strings.Contains(backUrl, url) && strings.Contains(backUrl, app.Instance.BaseURL) {
		url = fmt.Sprintf("%s#item-%s", backUrl, p.Hash)
	}
	p.Delete()
	if sav, ok := app.ContextItemSaver(r.Context()); ok {
		if _, err = sav.SaveItem(p); err != nil {
			addFlashMessage(Error, fmt.Sprintf("unable to add vote as an %s user", h.account.Handle), r)
		}
	}

	h.Redirect(w, r, url, http.StatusFound)
}

// HandleReport serves /{year}/{month}/{day}/{hash}/Report POST request
// HandleReport serves /~{handle}/Report request
func (h *handler) HandleReport(w http.ResponseWriter, r *http.Request) {
	m := contentModel{}
	h.RenderTemplate(r, w, "new", m)
}

// ShowReport serves /{year}/{month}/{day}/{hash}/Report GET request
// ShowReport serves /~{handle}/Report request
func (h *handler) ShowReport(w http.ResponseWriter, r *http.Request) {
	m := contentModel{}
	h.RenderTemplate(r, w, "new", m)
}

// HandleVoting serves /{year}/{month}/{day}/{hash}/{direction} request
// HandleVoting serves /~{handle}/{direction} request
func (h *handler) HandleVoting(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")

	val := r.Context().Value(app.RepositoryCtxtKey)
	itemLoader, ok := val.(app.CanLoadItems)
	if !ok {
		h.logger.Error("could not load item repository from Context")
		return
	}

	p, err := itemLoader.LoadItem(app.LoadItemsFilter{Key: []string{hash}})
	if err != nil {
		h.logger.Error(err.Error())
		h.HandleError(w, r, errors.NewNotFound(err, "not found"))
		return
	}

	direction := path.Base(r.URL.Path)
	multiplier := 0
	switch direction {
	case Yay:
		multiplier = 1
	case Nay:
		multiplier = -1
	}
	url := ItemPermaLink(p)

	acc := h.account
	if acc.IsLogged() {
		if auth, ok := val.(app.Authenticated); ok {
			auth.WithAccount(&acc)
		}
		voter, ok := val.(app.CanSaveVotes)
		backUrl := r.Header.Get("Referer")
		if !strings.Contains(backUrl, url) && strings.Contains(backUrl, app.Instance.BaseURL) {
			url = fmt.Sprintf("%s#item-%s", backUrl, p.Hash)
		}
		if !ok {
			h.logger.Error("could not load vote repository from Context")
			return
		}
		v := app.Vote{
			SubmittedBy: &acc,
			Item:        &p,
			Weight:      multiplier * app.ScoreMultiplier,
		}
		if _, err := voter.SaveVote(v); err != nil {
			h.logger.WithContext(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
			}).Error(err.Error())
		}
	} else {
		addFlashMessage(Error, fmt.Sprintf("unable to add vote as an %s user", acc.Handle), r)
	}
	h.Redirect(w, r, url, http.StatusFound)
}
