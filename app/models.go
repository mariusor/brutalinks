package app

import (
	"context"
)

type listingModel struct {
	Title          string
	User           *Account
	Items          []Renderable
	HideText       bool
	nextPage       int
	prevPage       int
}

func (i listingModel) NextPage() int {
	return i.nextPage
}

func (i listingModel) PrevPage() int {
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
