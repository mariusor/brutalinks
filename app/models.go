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
	Items    RenderableList
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
	Content Renderable
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
	if len(c.items) > 0 {
		m.Content = c.items[0]
	}
	m.after = c.after
	m.before = c.before
}

type loginModel struct {
	Title   string
	Account Account
	OAuth   bool
}

func (loginModel) Template() string {
	return "login"
}

func (*loginModel) SetCursor(c *Cursor) {}

type registerModel struct {
	Title   string
	Account Account
}

func (registerModel) Template() string {
	return "register"
}
func (*registerModel) SetCursor(c *Cursor) {}

type aboutModel struct {
	Title string
	Desc  Desc
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

func (errorModel) Template() string {
	return "error"
}
func (*errorModel) SetCursor(c *Cursor) {}
