package brutalinks

import (
	"html/template"
	"time"

	vocab "github.com/go-ap/activitypub"
)

type CursorSetter interface {
	SetCursor(*Cursor)
}

type Paginator interface {
	CursorSetter
	NextPage() vocab.IRI
	PrevPage() vocab.IRI
}

type Model interface {
	SetTitle(string)
	Template() string
}

type listingModel struct {
	Title        template.HTML
	tpl          string
	User         *Account
	ShowChildren bool
	children     RenderableList
	ShowText     bool
	after        vocab.IRI
	before       vocab.IRI
	sortFn       func(list RenderableList) []Renderable
}

func (m listingModel) AP() vocab.Item {
	return nil
}

func (m listingModel) IsValid() bool {
	return false
}

func (m listingModel) Type() RenderType {
	return UnknownType
}

func (m listingModel) Date() time.Time {
	return time.Now().UTC()
}

func (m listingModel) Children() *RenderableList {
	return &m.children
}

var _ Renderable = listingModel{}

func (m listingModel) ID() Hash {
	return Hash{}
}

func (m listingModel) NextPage() vocab.IRI {
	return m.after
}

func (m listingModel) PrevPage() vocab.IRI {
	return m.before
}

func (m *listingModel) SetCursor(c *Cursor) {
	if c == nil {
		return
	}
	m.children = c.items
	m.after = c.after
	m.before = c.before
}

func (m *listingModel) SetTitle(s string) {
	m.Title = template.HTML(s)
}

func (m listingModel) Template() string {
	if m.tpl == "" {
		m.tpl = "listing"
	}
	return m.tpl
}

func (m listingModel) Sorted() []Renderable {
	return m.sortFn(m.children)
}

type mBox struct {
	Readonly    bool
	Editable    bool
	ShowTitle   bool
	Label       template.HTML
	Hash        Hash
	OP          Hash
	Title       template.HTML
	Content     template.HTML
	Back        template.HTML
	SubmitLabel template.HTML
}

type contentModel struct {
	tpl          string
	Title        template.HTML
	Hash         Hash
	Content      Renderable
	ShowChildren bool
	Message      mBox
	after        vocab.IRI
	before       vocab.IRI
}

func (m contentModel) ID() Hash {
	return Hash{}
}

func (m contentModel) NextPage() vocab.IRI {
	return m.after
}

func (m contentModel) PrevPage() vocab.IRI {
	return m.before
}

func (m *contentModel) SetTitle(s string) {
	m.Title = template.HTML(s)
}

func (m contentModel) Template() string {
	if m.tpl == "" {
		m.tpl = "content"
	}
	return m.tpl
}

func (m contentModel) Level() uint8 {
	return 99
}

func getItemFromList(p Renderable, items RenderableList) Renderable {
	if p == nil {
		return nil
	}
	var findOrDescendFn func(collection RenderableList) (Renderable, bool)
	findOrDescendFn = func(collection RenderableList) (Renderable, bool) {
		for _, com := range collection {
			if p.ID() == com.ID() {
				return com, true
			}
			if cur, found := findOrDescendFn(*com.Children()); found {
				return cur, found
			}
		}
		return nil, false
	}

	for _, cur := range items {
		if p.ID() == cur.ID() {
			return cur
		}
		if cur, found := findOrDescendFn(*cur.Children()); found {
			return cur
		}
	}
	return nil
}

func (m *contentModel) SetCursor(c *Cursor) {
	if c == nil {
		return
	}
	m.after = c.after
	m.before = c.before
	if len(c.items) == 0 {
		return
	}
	if !m.Content.IsValid() {
		if m.Hash.IsValid() {
			m.Content = getItemFromList(&Item{Hash: m.Hash}, c.items)
		} else {
			for _, it := range c.items {
				m.Content = it
				break
			}
		}
	}
	if m.Content != nil {
		if it, ok := m.Content.(*Item); ok {
			if it.Private() {
				lbl := "Reply"
				if m.Message.Editable {
					lbl = "Edit"
				}
				m.Message.SubmitLabel = htmlf("%s %s", icon("lock"), lbl)
			}
		}
		m.Message.Back = htmlf("%s", PermaLink(m.Content))
	} else {
		missing := DeletedItem
		missing.Hash = m.Hash
		missing.children = c.items
		m.Content = &missing
	}
}

type moderationModel struct {
	Title        template.HTML
	Hash         Hash
	Content      *ModerationOp
	ShowChildren bool
	Message      mBox
	after        vocab.IRI
	before       vocab.IRI
}

func (m moderationModel) NextPage() vocab.IRI {
	return m.after
}

func (m moderationModel) PrevPage() vocab.IRI {
	return m.before
}

func (m *moderationModel) SetTitle(s string) {
	m.Title = template.HTML(s)
}

func (m moderationModel) Template() string {
	return "moderate"
}

func (m *moderationModel) SetCursor(c *Cursor) {
	if c == nil {
		return
	}
	m.after = c.after
	m.before = c.before
	if len(c.items) == 0 {
		return
	}
	if m.Content.Object == nil || !m.Content.Object.IsValid() {
		m.Content.Object = getItemFromList(&Item{Hash: m.Hash}, c.items)
	}
	if m.Content.Object != nil && len(m.Message.Back) == 0 {
		m.Message.Back = htmlf("%s", PermaLink(m.Content.Object))
	}
}

type loginModel struct {
	Title    template.HTML
	Account  Account
	Provider string
	OAuth    bool
}

func (m *loginModel) SetTitle(s string) {
	m.Title = template.HTML(s)
}

func (loginModel) Template() string {
	return "login"
}

func (*loginModel) SetCursor(c *Cursor) {}

type registerModel struct {
	Title   template.HTML
	Account Account
}

func (m *registerModel) SetTitle(s string) {
	m.Title = template.HTML(s)
}

func (registerModel) Template() string {
	return "register"
}
func (*registerModel) SetCursor(c *Cursor) {}

// Stats holds data for keeping compatibility with Mastodon instances
type Stats struct {
	DomainCount int  `json:"domain_count"`
	UserCount   uint `json:"user_count"`
	StatusCount uint `json:"status_count"`
}

// Desc holds data for keeping compatibility with Mastodon instances
type Desc struct {
	Description string   `json:"description"`
	Email       string   `json:"email"`
	Stats       Stats    `json:"stats"`
	Thumbnail   string   `json:"thumbnail,omitempty"`
	Title       string   `json:"title"`
	Lang        []string `json:"languages"`
	URI         string   `json:"uri"`
	Urls        []string `json:"urls,omitempty"`
	Version     string   `json:"version"`
}

type aboutModel struct {
	Title template.HTML
	Desc  Desc
}

func (m *aboutModel) SetTitle(s string) {
	m.Title = template.HTML(s)
}

func (m aboutModel) Template() string {
	return "about"
}

func (*aboutModel) SetCursor(c *Cursor) {}

type errorModel struct {
	Status     int
	StatusText string
	Title      template.HTML
	Errors     []error
}

func (m *errorModel) SetTitle(s string) {
	m.Title = template.HTML(s)
}

func (errorModel) Template() string {
	return "error"
}
func (*errorModel) SetCursor(c *Cursor) {}

func sortModel(m Model) func(list RenderableList) []Renderable {
	return func(list RenderableList) []Renderable {
		if list == nil {
			return nil
		}
		var sortFn = ByDate
		if lModel, ok := m.(*listingModel); ok && lModel.sortFn != nil {
			sortFn = lModel.sortFn
		}
		return sortFn(list)
	}
}
