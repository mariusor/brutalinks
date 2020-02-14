package app

type Paginator interface {
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

func (i listingModel) NextPage() Hash {
	return i.after
}

func (i listingModel) PrevPage() Hash {
	return i.before
}

func (i *listingModel) SetItems(items []Renderable) {
	i.Items = items
}

func (i listingModel) Template() string {
	if i.tpl == "" {
		i.tpl = "listing"
	}
	return i.tpl
}

type contentModel struct {
	tpl string
	Title   string
	Content Item
	after   Hash
	before  Hash
}

func (c contentModel) NextPage() Hash {
	return c.after
}

func (c contentModel) PrevPage() Hash {
	return c.before
}

func (c contentModel) Template() string {
	if c.tpl == "" {
		c.tpl = "content"
	}
	return c.tpl
}

type loginModel struct {
	Title   string
	Account Account
	OAuth   bool
}

func (c loginModel) Template() string {
	return "login"
}

type registerModel struct {
	Title   string
	Account Account
}

func (c registerModel) Template() string {
	return "register"
}

type aboutModel struct {
	Title string
	Desc  Desc
}

func (c aboutModel) Template() string {
	return "about"
}

type errorModel struct {
	Status int
	Title  string
	Errors []error
}

func (c errorModel) Template() string {
	return "error"
}
