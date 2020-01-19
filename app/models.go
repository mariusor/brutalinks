package app

import (
	"context"
)

type Paginator interface {
	NextPage() Hash
	PrevPage() Hash
}

type listingModel struct {
	Title    string
	User     *Account
	Items    []Renderable
	HideText bool
	after    Hash
	before   Hash
}

func (i listingModel) NextPage() Hash {
	return i.after
}

func (i listingModel) PrevPage() Hash {
	return i.before
}

type contentModel struct {
	Title    string
	Content  comment
	after    Hash
	before   Hash
}

func (c contentModel) NextPage() Hash {
	return c.after
}

func (c contentModel) PrevPage() Hash {
	return c.before
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

type aboutModel struct {
	Title string
	Desc  Desc
}

type errorModel struct {
	Status int
	Title  string
	Errors []error
}

func ListingModelFromContext(ctx context.Context) *listingModel {
	if l, ok := ctx.Value(CollectionCtxtKey).(*listingModel); ok {
		return l
	}
	return nil
}
