package app

type Paginator interface {
	SetCursor(*Cursor)
	NextPage() Hash
	PrevPage() Hash
}

type Model interface {
	Template() string
}

type listingModel struct {
	Title    string
	tpl      string
	User     *Account
	Items    []Renderable
	ShowText bool
	after    Hash
	before   Hash
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

func (m listingModel) Template() string {
	if m.tpl == "" {
		m.tpl = "listing"
	}
	return m.tpl
}

type contentModel struct {
	tpl     string
	Title   string
	Content Item
	after   Hash
	before  Hash
}

func (m contentModel) NextPage() Hash {
	return m.after
}

func (m contentModel) PrevPage() Hash {
	return m.before
}

func (m contentModel) Template() string {
	if m.tpl == "" {
		m.tpl = "content"
	}
	return m.tpl
}

func (m *contentModel) SetCursor(c *Cursor) {
	if c == nil {
		return
	}
	comments := make(ItemCollection, 0)
	for _, ren := range c.items {
		if it, ok := ren.(Item); ok {
			comments = append(comments, it)
		}
	}

	if len(comments) == 0 {
		return
	}

	reparentComments(comments)
	addLevelComments(comments)
	removeCurElementParentComments(&comments)

	m.Content = comments[0]
	m.Content.Children = comments[1:]

	m.after = c.after
	m.before = c.before
}

type loginModel struct {
	Title   string
	Account Account
	OAuth   bool
}

func (c loginModel) Template() string {
	return "login"
}

func (m *loginModel) SetCursor(c *Cursor) {}

type registerModel struct {
	Title   string
	Account Account
}

func (c registerModel) Template() string {
	return "register"
}
func (m *registerModel) SetCursor(c *Cursor) {}

type aboutModel struct {
	Title string
	Desc  Desc
}

func (m aboutModel) Template() string {
	return "about"
}
func (m *aboutModel) SetCursor(c *Cursor) {}

type errorModel struct {
	Status int
	Title  string
	Errors []error
}

func (m errorModel) Template() string {
	return "error"
}
func (m *errorModel) SetCursor(c *Cursor) {}
