package frontend

import (
	"fmt"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/mariusor/qstring"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/mariusor/littr.go/app"

	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
)

const Edit = "edit"
const Delete = "rm"
const Report = "bad"
const Yay = "yay"
const Nay = "nay"

type comments []*comment
type comment struct {
	app.Item
	// Voted shows if current logged account has Yayed(+1) or Nayed(-1) current Item
	Voted    uint8
	Level    uint8
	Edit     bool
	Children comments
	Parent   *comment
}

func (c *comment) Type() RenderType {
	return Comment
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
	var all = make(comments, len(items))
	for k, item := range items {
		com := comment{Item: item}
		all[k] = &com
	}
	return all
}

func (c comments) Contains(cc comment) bool {
	for _, com := range c {
		if app.HashesEqual(com.Hash, cc.Hash) {
			return true
		}
	}
	return false
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

	if t.Type == app.TagTag {
		cls = "tag"
	}
	if t.Type == app.TagMention {
		cls = "mention"
	}

	return fmt.Sprintf("<a href='%s' class='%s'>%s</a>", t.URL, cls, t.Name)
}

func inRange(n string, nn map[string]string) bool {
	for k := range nn {
		if k == n {
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
	replaces := make(map[string]string, 0)
	if cur.Metadata.Tags != nil {
		for _, t := range cur.Metadata.Tags {
			name := fmt.Sprintf("#%s", t.Name)
			if inRange(name, replaces) {
				continue
			}
			replaces[name] = mimeTypeTagReplace(cur.MimeType, t)
		}
	}
	if cur.Metadata.Mentions != nil {
		for idx, t := range cur.Metadata.Mentions {
			lbl := fmt.Sprintf(":::MENTION_%d:::", idx)
			if inRange(lbl, replaces) {
				continue
			}
			if u, err := url.Parse(t.URL); err == nil && len(u.Host) > 0 {
				nameAtT := fmt.Sprintf("~%s@%s", t.Name, u.Host)
				nameAtA := fmt.Sprintf("@%s@%s", t.Name, u.Host)
				dat = strings.ReplaceAll(dat, nameAtT, lbl)
				dat = strings.ReplaceAll(dat, nameAtA, lbl)
			}
			nameT := fmt.Sprintf("~%s", t.Name)
			nameA := fmt.Sprintf("@%s", t.Name)
			dat = strings.ReplaceAll(dat, nameT, lbl)
			dat = strings.ReplaceAll(dat, nameA, lbl)
			replaces[lbl] = mimeTypeTagReplace(cur.MimeType, t)
		}
	}

	for to, repl := range replaces {
		dat = strings.ReplaceAll(dat, to, repl)
	}
	return dat
}

func removeCurElementParentComments(com *comments) {
	first := (*com)[0]
	lvl := first.Level
	keepComments := make(comments, 0)
	for _, cur := range *com {
		if cur.Level >= lvl {
			keepComments = append(keepComments, cur)
		}
	}
	*com = keepComments
}

func addLevelComments(allComments comments) {
	leveled := make(app.Hashes, 0)
	var setLevel func(comments)

	setLevel = func(com comments) {
		for _, cur := range com {
			if leveled.Contains(cur.Hash) {
				break
			}
			leveled = append(leveled, cur.Hash)
			if len(cur.Children) > 0 {
				for _, child := range cur.Children {
					child.Level = cur.Level + 1
					setLevel(cur.Children)
				}
			}
		}
	}
	setLevel(allComments)
}

func reparentComments(allComments []*comment) {
	parFn := func(t []*comment, cur comment) *comment {
		for _, n := range t {
			if cur.Item.Parent.IsValid() {
				if app.HashesEqual(cur.Item.Parent.Hash, n.Hash) {
					return n
				}
			}
		}
		return nil
	}

	first := allComments[0]
	for _, cur := range allComments {
		if par := parFn(allComments, *cur); par != nil {
			if app.HashesEqual(first.Hash, cur.Hash) {
				continue
			}
			cur.Parent = par
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
	acctLoader, ok := app.ContextAccountLoader(r.Context())
	if !ok {
		h.logger.Error("could not load item repository from Context")
		return
	}
	handle := chi.URLParam(r, "handle")
	auth, err := acctLoader.LoadAccount(app.Filters{LoadAccountsFilter: app.LoadAccountsFilter{
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
			Key: app.Hashes{app.Hash(hash)},
		},
	}
	if !app.HashesEqual(auth.Hash, app.AnonymousHash) {
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

	account := h.account(r)
	if maybeEdit != hash && maybeEdit == Edit {
		if !app.HashesEqual(m.Content.SubmittedBy.Hash, account.Hash) {
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
			Depth: 10,
		},
		MaxItems: MaxContentItems,
		Page:     1,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}

	if i.OP.IsValid() {
		if id, ok := BuildIDFromItem(*i.OP); ok {
			filter.Context = []string{string(id)}
		}
	}
	if filter.Context == nil {
		filter.Context = []string{m.Content.Hash.String()}
	}
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

	if i.Parent.IsValid() && i.Parent.SubmittedAt.IsZero() {
		if p, err := itemLoader.LoadItem(app.Filters{LoadItemsFilter: app.LoadItemsFilter{Key: app.Hashes{i.Parent.Hash}}}); err == nil {
			i.Parent = &p
			if p.OP != nil {
				i.OP = p.OP
			} else {
				i.OP = &p
			}
		}
	}

	reparentComments(allComments)
	addLevelComments(allComments)
	removeCurElementParentComments(&allComments)

	if ok && account.IsLogged() {
		votesLoader, ok := app.ContextVoteLoader(r.Context())
		if ok {
			account.Votes, _, err = votesLoader.LoadVotes(app.Filters{
				LoadVotesFilter: app.LoadVotesFilter{
					AttributedTo: []app.Hash{account.Hash},
					ItemKey:      allComments.getItemsHashes(),
				},
				MaxItems: MaxContentItems,
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

func accountFromRequestHandle(r *http.Request) (*app.Account, error) {
	handle := chi.URLParam(r, "handle")
	accountLoader, ok := app.ContextAccountLoader(r.Context())
	if !ok {
		return nil, errors.Newf("could not load account repository from Context")
	}
	var err error
	accounts, cnt, err := accountLoader.LoadAccounts(app.Filters{LoadAccountsFilter: app.LoadAccountsFilter{Handle: []string{handle}}})
	if err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, errors.NotFoundf("account %q not found", handle)
	}
	if cnt > 1 {
		return nil, errors.NotFoundf("too many %q accounts found", handle)
	}
	return accounts.First()
}

// HandleSubmit handles POST /submit requests
// HandleSubmit handles POST /~handler/hash requests
// HandleSubmit handles POST /year/month/day/hash requests
// HandleSubmit handles POST /~handler/hash/edit requests
// HandleSubmit handles POST /year/month/day/hash/edit requests
func (h *handler) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	acc := h.account(r)
	n, err := ContentFromRequest(r, *acc)
	if err != nil {
		h.logger.WithContext(log.Ctx{
			"prev": err,
		}).Error("wrong http method")
		h.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	saveVote := true

	itemLoader, ok := app.ContextItemLoader(r.Context())
	if !ok {
		h.HandleErrors(w, r, errors.Errorf("could not load item repository from Context"))
		return
	}
	if n.Parent.IsValid() {
		if n.Parent.SubmittedAt.IsZero() {
			if p, err := itemLoader.LoadItem(app.Filters{LoadItemsFilter: app.LoadItemsFilter{Key: app.Hashes{n.Parent.Hash}}}); err == nil {
				n.Parent = &p
				if p.OP != nil {
					n.OP = p.OP
				}
			}
		}
		if len(n.Metadata.To) == 0 {
			n.Metadata.To = make([]*app.Account, 0)
		}
		n.Metadata.To = append(n.Metadata.To, n.Parent.SubmittedBy)
		if n.Parent.Private() {
			n.MakePrivate()
			saveVote = false
		}
	}

	if len(n.Hash) > 0 {
		if p, err := itemLoader.LoadItem(app.Filters{LoadItemsFilter: app.LoadItemsFilter{Key: app.Hashes{n.Hash}}}); err == nil {
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
		h.HandleErrors(w, r, err)
		return
	}

	if saveVote {
		var voteSaver app.CanSaveVotes
		if voteSaver, ok = app.ContextVoteSaver(r.Context()); !ok {
			h.logger.Error("could not load item repository from Context")
		}
		v := app.Vote{
			SubmittedBy: acc,
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
	p, err := itemLoader.LoadItem(app.Filters{LoadItemsFilter: app.LoadItemsFilter{Key: app.Hashes{app.Hash(hash)}}})
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

	acc := h.account(r)
	if acc.IsLogged() {
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
			SubmittedBy: acc,
			Item:        &p,
			Weight:      multiplier * app.ScoreMultiplier,
		}
		if _, err := voter.SaveVote(v); err != nil {
			h.logger.WithContext(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
				"error":  err,
			}).Error("Unable to save vote")
			h.addFlashMessage(Error, r, err.Error())
		}
	} else {
		h.addFlashMessage(Error, r, "unable to vote as current user")
	}
	h.Redirect(w, r, url, http.StatusFound)
}
