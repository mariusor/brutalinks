package frontend

import (
	"fmt"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/mariusor/qstring"
	"math"
	"net/http"
	"path"
	"strings"

	"github.com/mariusor/littr.go/app"

	"github.com/go-chi/chi"
	"github.com/mariusor/littr.go/internal/errors"
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
	Title    string
	Content  comment
	nextPage int
	prevPage int
}

func (c contentModel) NextPage() int {
	return c.nextPage
}

func (c contentModel) PrevPage() int {
	return c.prevPage
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

func (c comments) getItemsHashes() app.Hashes {
	var items = make(app.Hashes, len(c))
	for k, com := range c {
		items[k] = com.Item.Hash
	}
	return items
}

func sluggify(s app.MimeType) string {
	if s == "" {
		return ""
	}
	return strings.Replace(string(s), "/", "-", -1)
}

func mimeTypeTagReplace(m app.MimeType, t app.Tag) string {
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
			cur.Parent = par
			par.Children = append(par.Children, cur)
		}
	}

	// Append remaining non parented elements to parent element
	for i, cur := range allComments {
		if i == 0 {
			continue
		}
		if cur.Parent == nil {
			cur.Parent = allComments[0]
			cur.Parent.Children = append(cur.Parent.Children, cur)
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
	acctLoader, ok := app.ContextAccountLoader(r.Context())
	if !ok {
		h.logger.Error("could not load item repository from Context")
		return
	}
	handle := chi.URLParam(r, "handle")
	auth, err := acctLoader.LoadAccount(app.Filters{ LoadAccountsFilter: app.LoadAccountsFilter{
		Handle: []string{handle},
	}})
	itemLoader, ok := app.ContextItemLoader(r.Context())
	if !ok {
		h.logger.Error("could not load item repository from Context")
		return
	}

	hash := chi.URLParam(r, "hash")
	f := app.Filters{
		LoadItemsFilter: app.LoadItemsFilter{
			Key:          app.Hashes{app.Hash(hash)},
		},
	}
	if auth.Hash.String() != app.AnonymousHash.String() {
		f.LoadItemsFilter.AttributedTo = app.Hashes{auth.Hash}
	}
	i, err := itemLoader.LoadItem(f)

	if err != nil {
		h.logger.WithContext(log.Ctx{
			"handle": handle,
			"hash":   hash,
		}).Error(err.Error())
		h.HandleErrors(w, r, errors.NotFoundf("Item %q", hash))
		return
	}
	if !i.Deleted() && len(i.Data)+len(i.Title) == 0 {
		datLen := int(math.Min(12.0, float64(len(i.Data))))
		h.logger.WithContext(log.Ctx{
			"handle":      handle,
			"hash":        hash,
			"title":       i.Title,
			"content":     i.Data[0:datLen],
			"content_len": len(i.Data),
		}).Warn("Item deleted or empty")
		h.HandleErrors(w, r, errors.NotFoundf("Item %q", hash))
		return
	}
	m.Content = comment{Item: i}
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

	filter := app.Filters{
		LoadItemsFilter: app.LoadItemsFilter{
			Depth:    10,
		},
		MaxItems: MaxContentItems,
		Page:     1,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}
	filter.Context = []string{m.Content.Hash.String()}
	contentItems, _, err := itemLoader.LoadItems(filter)
	if len(contentItems) >= filter.MaxItems {
		m.nextPage = filter.Page + 1
	}
	if filter.Page > 1 {
		m.prevPage = filter.Page - 1
	}
	if err != nil {
		h.logger.Error(err.Error())
		h.HandleErrors(w, r, errors.NewNotFound(err, "" /*, errors.ErrorStack(err)*/))
		return
	}
	allComments = append(allComments, loadComments(contentItems)...)

	//replaceTags(allComments)
	reparentComments(allComments)
	addLevelComments(allComments)

	if ok && h.account.IsLogged() {
		votesLoader, ok := app.ContextVoteLoader(r.Context())
		if ok {
			h.account.Votes, _, err = votesLoader.LoadVotes(app.Filters{
				LoadVotesFilter: app.LoadVotesFilter{
					AttributedTo: []app.Hash{h.account.Hash},
					ItemKey:      allComments.getItemsHashes(),
				},
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

// HandleSubmit handles POST /submit requests
// HandleSubmit handles POST /~handler/hash requests
// HandleSubmit handles POST /year/month/day/hash requests
// HandleSubmit handles POST /~handler/hash/edit requests
// HandleSubmit handles POST /year/month/day/hash/edit requests
func (h *handler) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	n, err := ContentFromRequest(r, h.account)
	if err != nil {
		h.logger.WithContext(log.Ctx{
			"prev": err,
		}).Error("wrong http method")
		h.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}

	acc := h.account
	auth, authOk := app.ContextAuthenticated(r.Context())
	if authOk && acc.IsLogged() {
		auth.WithAccount(&acc)
	}
	val := r.Context().Value(app.RepositoryCtxtKey)
	itemLoader, ok := val.(app.CanLoadItems)
	if !ok {
		h.HandleErrors(w, r, errors.Errorf("could not load item repository from Context"))
		return
	}
	saveVote := true
	if len(n.Hash) > 0 {
		if p, err := itemLoader.LoadItem(app.Filters{ LoadItemsFilter: app.LoadItemsFilter{Key: app.Hashes{n.Hash}}}); err == nil {
			n.Title = p.Title
		}
		saveVote = false
	}

	var itemSaver app.CanSaveItems
	if itemSaver, ok = app.ContextItemSaver(r.Context()); !ok {
		h.logger.Error("could not load item repository from Context")
		return
	}
	n, err = itemSaver.SaveItem(n)
	if err != nil {
		h.logger.WithContext(log.Ctx{
			"prev": err,
		}).Error("unable to save item")
		h.HandleErrors(w, r, errors.NewNotValid(err, "oops!"))
		return
	}

	if saveVote {
		var voteSaver app.CanSaveVotes
		if voteSaver, ok = app.ContextVoteSaver(r.Context()); !ok {
			h.logger.Error("could not load item repository from Context")
		}
		v := app.Vote{
			SubmittedBy: &acc,
			Item:        &n,
			Weight:      1 * app.ScoreMultiplier,
		}
		if _, err := voteSaver.SaveVote(v); err != nil {
			h.logger.WithContext(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
			}).Error(err.Error())
		}
	}
	h.Redirect(w, r, ItemPermaLink(n), http.StatusSeeOther)
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

// HandleDelete serves /{year}/{month}/{day}/{hash}/rm POST request
// HandleDelete serves /~{handle}/rm GET request
func (h *handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")

	val := r.Context().Value(app.RepositoryCtxtKey)
	itemLoader, ok := val.(app.CanLoadItems)
	if !ok {
		h.logger.Error("could not load item repository from Context")
		return
	}
	p, err := itemLoader.LoadItem(app.Filters{ LoadItemsFilter: app.LoadItemsFilter{Key: app.Hashes{app.Hash(hash)}}})
	if err != nil {
		h.logger.Error(err.Error())
		h.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}

	url := ItemPermaLink(p)
	backUrl := r.Header.Get("Referer")
	if !strings.Contains(backUrl, url) && strings.Contains(backUrl, app.Instance.BaseURL) {
		url = fmt.Sprintf("%s#item-%s", backUrl, p.Hash)
	}
	acc := h.account
	auth, authOk := app.ContextAuthenticated(r.Context())
	if authOk && acc.IsLogged() {
		auth.WithAccount(&acc)
	}
	p.Delete()
	if sav, ok := app.ContextItemSaver(r.Context()); ok {
		if _, err = sav.SaveItem(p); err != nil {
			h.addFlashMessage(Error, r, "unable to delete item as current user")
		}
	}

	h.Redirect(w, r, url, http.StatusFound)
}

// HandleReport serves /{year}/{month}/{day}/{hash}/report POST request
// HandleReport serves /~{handle}/report request
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

	p, err := itemLoader.LoadItem(app.Filters{LoadItemsFilter: app.LoadItemsFilter{Key: app.Hashes{app.Hash(hash)}}})
	if err != nil {
		h.logger.Error(err.Error())
		h.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}

	direction := path.Base(r.URL.Path)
	multiplier := 0
	switch strings.ToLower(direction) {
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
		h.addFlashMessage(Error, r, "unable to vote as current user")
	}
	h.Redirect(w, r, url, http.StatusFound)
}
