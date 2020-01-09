package app

import (
	"database/sql/driver"
	"fmt"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/mariusor/qstring"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"

	mark "gitlab.com/golang-commonmark/markdown"
)

type FlagBits uint8

const (
	FlagsDeleted = FlagBits(1 << iota)
	FlagsPrivate

	FlagsNone = FlagBits(0)
)

const MimeTypeURL = MimeType("application/url")
const MimeTypeHTML = MimeType("text/html")
const MimeTypeMarkdown = MimeType("text/markdown")
const MimeTypeText = MimeType("text/plain")
const RandomSeedSelectedByDiceRoll = 777

type Key [32]byte

func (k Key) IsEmpty() bool {
	return k == Key{}
}

func (k Key) String() string {
	return string(k[0:32])
}
func (k Key) Bytes() []byte {
	return []byte(k[0:32])
}
func (k Key) Hash() Hash {
	return Hash(k[0:32])
}

func (k *Key) FromBytes(s []byte) error {
	var err error
	if len(s) > 32 {
		err = errors.Errorf("incoming byte array %q longer than expected ", s)
	}
	if len(s) < 32 {
		err = errors.Errorf("incoming byte array %q longer than expected ", s)
	}
	for i := range s {
		k[i] = s[i]
	}
	return err
}
func (k *Key) FromString(s string) error {
	var err error
	if len(s) > 32 {
		err = errors.Errorf("incoming string %q longer than expected ", s)
	}
	if len(s) < 32 {
		err = errors.Errorf("incoming string %q longer than expected ", s)
	}
	for i := range s {
		k[i] = s[i]
	}
	return err
}

// Value implements the driver.Valuer interface
func (k Key) Value() (driver.Value, error) {
	if len(k) > 0 {
		return k.String(), nil
	}
	return nil, nil
}

// Scan implements the sql.Scanner interface
func (k *Key) Scan(src interface{}) error {
	if v, ok := src.([]byte); ok {
		k.FromBytes(v)
	} else {
		return errors.Errorf("bad []byte type assertion when loading %T", k)
	}

	return nil
}
func (f *FlagBits) FromInt64() error {
	return nil
}

type ItemCollection []Item

func Markdown(data string) template.HTML {
	md := mark.New(
		mark.HTML(true),
		mark.Tables(true),
		mark.Linkify(false),
		mark.Breaks(false),
		mark.Typographer(true),
		mark.XHTMLOutput(false),
	)

	h := md.RenderToString([]byte(data))
	return template.HTML(h)
}

// HasMetadata
func (i *Item) HasMetadata() bool {
	return i != nil && i.Metadata != nil
}

// IsFederated
func (i Item) IsFederated() bool {
	return !i.IsLocal()
}

// IsLocal
func (i Item) IsLocal() bool {
	if !i.HasMetadata() {
		return true
	}
	if len(i.Metadata.ID) > 0 {
		return HostIsLocal(i.Metadata.ID)
	}
	if len(i.Metadata.URL) > 0 {
		return HostIsLocal(i.Metadata.URL)
	}
	return true
}

const Edit = "edit"
const Delete = "rm"
const Report = "bad"
const Yay = "yay"
const Nay = "nay"

type RenderType int

const (
	Comment = iota
	Follow
	Appreciation
)

type HasType interface {
	Type() RenderType
}

type follow struct {
	FollowRequest
}

func (f *follow) Type() RenderType {
	return Follow
}

type comments []*comment

type comment struct {
	Item
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

func loadComments(items []Item) comments {
	var all = make(comments, len(items))
	for k, item := range items {
		com := comment{Item: item}
		all[k] = &com
	}
	return all
}

func (c comments) Contains(cc comment) bool {
	for _, com := range c {
		if HashesEqual(com.Hash, cc.Hash) {
			return true
		}
	}
	return false
}

func (c comments) getItems() ItemCollection {
	var items = make(ItemCollection, len(c))
	for k, com := range c {
		items[k] = com.Item
	}
	return items
}

func (c comments) getItemsHashes() Hashes {
	var items = make(Hashes, len(c))
	for k, com := range c {
		items[k] = com.Item.Hash
	}
	return items
}

func sluggify(s MimeType) string {
	if s == "" {
		return ""
	}
	return strings.Replace(string(s), "/", "-", -1)
}

func mimeTypeTagReplace(m MimeType, t Tag) string {
	var cls string

	if t.Type == TagTag {
		cls = "tag"
	}
	if t.Type == TagMention {
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

func replaceTagsInItem(cur Item) string {
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
	leveled := make(Hashes, 0)
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
				if HashesEqual(cur.Item.Parent.Hash, n.Hash) {
					return n
				}
			}
		}
		return nil
	}

	first := allComments[0]
	for _, cur := range allComments {
		if par := parFn(allComments, *cur); par != nil {
			if HashesEqual(first.Hash, cur.Hash) {
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
	items := make([]Item, 0)

	m := contentModel{}
	repo := h.storage
	handle := chi.URLParam(r, "handle")
	auth, err := repo.LoadAccount(Filters{LoadAccountsFilter: LoadAccountsFilter{
		Handle: []string{handle},
	}})

	hash := chi.URLParam(r, "hash")
	f := Filters{
		LoadItemsFilter: LoadItemsFilter{
			Key: Hashes{Hash(hash)},
		},
	}
	if !HashesEqual(auth.Hash, AnonymousHash) {
		f.LoadItemsFilter.AttributedTo = Hashes{auth.Hash}
	}

	i, err := repo.LoadItem(f)
	if err != nil {
		h.logger.WithContext(log.Ctx{
			"handle": handle,
			"hash":   hash,
		}).Error(err.Error())
		h.v.HandleErrors(w, r, errors.NotFoundf("Item %q", hash))
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
		h.v.HandleErrors(w, r, errors.NotFoundf("Item %q", hash))
		return
	}
	m.Content = comment{Item: i}
	url := r.URL
	maybeEdit := path.Base(url.Path)

	account := account(r)
	if maybeEdit != hash && maybeEdit == Edit {
		if !HashesEqual(m.Content.SubmittedBy.Hash, account.Hash) {
			url.Path = path.Dir(url.Path)
			h.v.Redirect(w, r, url.RequestURI(), http.StatusFound)
			return
		}
		m.Content.Edit = true
	}

	items = append(items, i)
	allComments := make(comments, 1)
	allComments[0] = &m.Content

	filter := Filters{
		LoadItemsFilter: LoadItemsFilter{
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
	contentItems, _, err := repo.LoadItems(filter)
	if len(contentItems) >= filter.MaxItems {
		m.nextPage = filter.Page + 1
	}
	if filter.Page > 1 {
		m.prevPage = filter.Page - 1
	}
	if err != nil {
		h.logger.Error(err.Error())
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "" /*, errors.ErrorStack(err)*/))
		return
	}
	allComments = append(allComments, loadComments(contentItems)...)

	if i.Parent.IsValid() && i.Parent.SubmittedAt.IsZero() {
		if p, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{i.Parent.Hash}}}); err == nil {
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

	if account.IsLogged() {
		account.Votes, _, err = repo.LoadVotes(Filters{
			LoadVotesFilter: LoadVotesFilter{
				AttributedTo: []Hash{account.Hash},
				ItemKey:      allComments.getItemsHashes(),
			},
			MaxItems: MaxContentItems,
		})
		if err != nil {
			h.logger.Error(err.Error())
		}
	}

	if len(m.Title) > 0 {
		m.Title = fmt.Sprintf("%s", i.Title)
	} else {
		// FIXME(marius): we lost the handler of the account
		m.Title = fmt.Sprintf("%s comment", genitive(m.Content.SubmittedBy.Handle))
	}
	h.v.RenderTemplate(r, w, "content", m)
}

func accountFromRequestHandle(r *http.Request) (*Account, error) {
	handle := chi.URLParam(r, "handle")
	repo, ok := ContextRepository(r.Context())
	if !ok {
		return nil, errors.Newf("could not load account repository from Context")
	}
	var err error
	accounts, cnt, err := repo.LoadAccounts(Filters{LoadAccountsFilter: LoadAccountsFilter{Handle: []string{handle}}})
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
	acc := account(r)
	n, err := ContentFromRequest(r, *acc)
	if err != nil {
		h.logger.WithContext(log.Ctx{
			"prev": err,
		}).Error("wrong http method")
		h.v.HandleErrors(w, r, errors.NewMethodNotAllowed(err, ""))
		return
	}
	saveVote := true

	repo := h.storage
	if n.Parent.IsValid() {
		if n.Parent.SubmittedAt.IsZero() {
			if p, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{n.Parent.Hash}}}); err == nil {
				n.Parent = &p
				if p.OP != nil {
					n.OP = p.OP
				}
			}
		}
		if len(n.Metadata.To) == 0 {
			n.Metadata.To = make([]*Account, 0)
		}
		n.Metadata.To = append(n.Metadata.To, n.Parent.SubmittedBy)
		if n.Parent.Private() {
			n.MakePrivate()
			saveVote = false
		}
	}

	if len(n.Hash) > 0 {
		if p, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{n.Hash}}}); err == nil {
			n.Title = p.Title
		}
		saveVote = false
	}
	n, err = repo.SaveItem(n)
	if err != nil {
		h.logger.WithContext(log.Ctx{
			"prev": err,
		}).Error("unable to save item")
		h.v.HandleErrors(w, r, err)
		return
	}

	if saveVote {
		v := Vote{
			SubmittedBy: acc,
			Item:        &n,
			Weight:      1 * ScoreMultiplier,
		}
		if _, err := repo.SaveVote(v); err != nil {
			h.logger.WithContext(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
			}).Error(err.Error())
		}
	}
	h.v.Redirect(w, r, ItemPermaLink(n), http.StatusSeeOther)
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

	repo := h.storage
	p, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{Hash(hash)}}})
	if err != nil {
		h.logger.Error(err.Error())
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
		return
	}

	url := ItemPermaLink(p)
	backUrl := r.Header.Get("Referer")
	if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL) {
		url = fmt.Sprintf("%s#item-%s", backUrl, p.Hash)
	}
	p.Delete()
	if p, err = repo.SaveItem(p); err != nil {
		h.v.addFlashMessage(Error, r, "unable to delete item as current user")
	}

	h.v.Redirect(w, r, url, http.StatusFound)
}

// HandleReport serves /{year}/{month}/{day}/{hash}/report POST request
// HandleReport serves /~{handle}/report request
func (h *handler) HandleReport(w http.ResponseWriter, r *http.Request) {
	m := contentModel{}
	h.v.RenderTemplate(r, w, "new", m)
}

// ShowReport serves /{year}/{month}/{day}/{hash}/Report GET request
// ShowReport serves /~{handle}/Report request
func (h *handler) ShowReport(w http.ResponseWriter, r *http.Request) {
	m := contentModel{}
	h.v.RenderTemplate(r, w, "new", m)
}

// HandleVoting serves /{year}/{month}/{day}/{hash}/{direction} request
// HandleVoting serves /~{handle}/{direction} request
func (h *handler) HandleVoting(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")

	repo := h.storage
	p, err := repo.LoadItem(Filters{LoadItemsFilter: LoadItemsFilter{Key: Hashes{Hash(hash)}}})
	if err != nil {
		h.logger.Error(err.Error())
		h.v.HandleErrors(w, r, errors.NewNotFound(err, "not found"))
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

	acc := account(r)
	if acc.IsLogged() {
		backUrl := r.Header.Get("Referer")
		if !strings.Contains(backUrl, url) && strings.Contains(backUrl, Instance.BaseURL) {
			url = fmt.Sprintf("%s#item-%s", backUrl, p.Hash)
		}
		v := Vote{
			SubmittedBy: acc,
			Item:        &p,
			Weight:      multiplier * ScoreMultiplier,
		}
		if _, err := repo.SaveVote(v); err != nil {
			h.logger.WithContext(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
				"error":  err,
			}).Error("Unable to save vote")
			h.v.addFlashMessage(Error, r, err.Error())
		}
	} else {
		h.v.addFlashMessage(Error, r, "unable to vote as current user")
	}
	h.v.Redirect(w, r, url, http.StatusFound)
}
