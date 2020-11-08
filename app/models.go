package app

import (
	"html/template"
)

type Paginator interface {
	SetCursor(*Cursor)
	NextPage() Hash
	PrevPage() Hash
}

type Model interface {
	SetTitle(string)
	Template() string
}

type listingModel struct {
	Title    string
	tpl      string
	User     *Account
	Items    RenderableList
	ShowText bool
	after    Hash
	before   Hash
	sortFn   func(list RenderableList) []Renderable
}

func (m listingModel) NextPage() Hash {
	return m.after
}

func (m listingModel) PrevPage() Hash {
	return m.before
}

func (m *listingModel) SetCursor(c *Cursor) {
	if c == nil {
		return
	}
	m.Items = c.items
	m.after = c.after
	m.before = c.before
}

func (m *listingModel) SetTitle(s string) {
	m.Title = s
}

func (m listingModel) Template() string {
	if m.tpl == "" {
		m.tpl = "listing"
	}
	return m.tpl
}

func (m listingModel) Sorted() []Renderable {
	return m.sortFn(m.Items)
}

type mBox struct {
	Readonly    bool
	Editable    bool
	ShowTitle   bool
	Label       string
	Hash        Hash
	OP          Hash
	Title       string
	Back        string
	SubmitLabel template.HTML
}

type contentModel struct {
	tpl          string
	Title        string
	Hash         Hash
	Content      Renderable
	ShowChildren bool
	Message      mBox
	after        Hash
	before       Hash
}

func (m contentModel) NextPage() Hash {
	return m.after
}

func (m contentModel) PrevPage() Hash {
	return m.before
}

func (m *contentModel) SetTitle(s string) {
	m.Title = s
}

func (m contentModel) Template() string {
	if m.tpl == "" {
		m.tpl = "content"
	}
	return m.tpl
}

func getFromList(h Hash, items RenderableList) *Item {
	var findOrDescendFn func(collection ItemPtrCollection) (*Item, bool)
	findOrDescendFn = func(collection ItemPtrCollection) (*Item, bool) {
		for _, com := range collection {
			if h == com.Hash {
				return com, true
			}
			if cur, found := findOrDescendFn(com.children); found {
				return cur, found
			}
		}
		return nil, false
	}

	for _, cur := range items {
		if cur.Type() != CommentType {
			continue
		}
		if it, ok := cur.(*Item); ok {
			if h == it.Hash {
				return it
			}
			if cur, found := findOrDescendFn(it.children); found {
				return cur
			}
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
		m.Content = getFromList(m.Hash, c.items)
	}
	if m.Content != nil {
		if it, ok := m.Content.(*Item); ok {
			if it.Private() {
				m.Message.SubmitLabel = htmlf("%s Reply", icon("lock"))
			}
		}
		m.Message.Back = PermaLink(m.Content)
	}
}

type moderationModel struct {
	Title        string
	Hash         Hash
	Content      *ModerationOp
	ShowChildren bool
	Message      mBox
	after        Hash
	before       Hash
}

func (m moderationModel) NextPage() Hash {
	return m.after
}

func (m moderationModel) PrevPage() Hash {
	return m.before
}

func (m *moderationModel) SetTitle(s string) {
	m.Title = s
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
		m.Content.Object = getFromList(m.Hash, c.items)
	}
	if m.Content.Object != nil && len(m.Message.Back) == 0 {
		m.Message.Back = PermaLink(m.Content.Object)
	}
}

type loginModel struct {
	Title   string
	Account Account
	OAuth   bool
}

func (m *loginModel) SetTitle(s string) {
	m.Title = s
}

func (loginModel) Template() string {
	return "login"
}

func (*loginModel) SetCursor(c *Cursor) {}

type registerModel struct {
	Title   string
	Account Account
}

func (m *registerModel) SetTitle(s string) {
	m.Title = s
}

func (registerModel) Template() string {
	return "register"
}
func (*registerModel) SetCursor(c *Cursor) {}

type aboutModel struct {
	Title string
	Desc  Desc
}

func (m *aboutModel) SetTitle(s string) {
	m.Title = s
}

func (m aboutModel) Template() string {
	return "about"
}

func (*aboutModel) SetCursor(c *Cursor) {}

type errorModel struct {
	Status     int
	StatusText string
	Title      string
	Errors     []error
}

func (m *errorModel) SetTitle(s string) {
	m.Title = s
}

func (errorModel) Template() string {
	return "error"
}
func (*errorModel) SetCursor(c *Cursor) {}
