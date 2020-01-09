package app

type itemListingModel struct {
	Title          string
	User           *Account
	Items          []HasType
	HideText       bool
	nextPage       int
	prevPage       int
}

func (i itemListingModel) NextPage() int {
	return i.nextPage
}

func (i itemListingModel) PrevPage() int {
	return i.prevPage
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

type loginModel struct {
	Title   string
	Account Account
	OAuth   bool
}

type registerModel struct {
	Title   string
	Account Account
}

type errorModel struct {
	Status int
	Title  string
	Errors []error
}
