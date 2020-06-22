package app

import (
	"context"
	"strings"
)

type CtxtKey string

var (
	ServicesCtxtKey      CtxtKey = "__di"
	LoggedAccountCtxtKey CtxtKey = "__acct"
	RepositoryCtxtKey    CtxtKey = "__repository"
	FilterCtxtKey        CtxtKey = "__filter"
	ModelCtxtKey         CtxtKey = "__model"
	AuthorCtxtKey        CtxtKey = "__author"
	CursorCtxtKey        CtxtKey = "__cursor"
	ContentCtxtKey       CtxtKey = "__content"
)

type WebInfo struct {
	Title       string   `json:"title"`
	Email       string   `json:"email"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Thumbnail   string   `json:"thumbnail,omitempty"`
	Languages   []string `json:"languages"`
	URI         string   `json:"uri"`
	Urls        []string `json:"urls,omitempty"`
	Version     string   `json:"version"`
}

type Filterable interface {
	GetWhereClauses() ([]string, []interface{})
	GetLimit() string
}

type Hashes []Hash

func (h Hashes) Contains(s Hash) bool {
	for _, hh := range h {
		if HashesEqual(hh, s) {
			return true
		}
	}
	return false
}

func (h Hashes) String() string {
	str := make([]string, len(h))
	for i, hh := range h {
		str[i] = string(hh)
	}
	return strings.Join(str, ", ")
}

func ContextModel(ctx context.Context) Model {
	var m Model
	m, _ = ctx.Value(ModelCtxtKey).(Model)
	return m
}

func ContextListingModel(ctx context.Context) *listingModel {
	var m *listingModel
	m, _ = ctx.Value(ModelCtxtKey).(*listingModel)
	return m
}

func ContextContentModel(ctx context.Context) *contentModel {
	var m *contentModel
	m, _ = ctx.Value(ModelCtxtKey).(*contentModel)
	return m
}

func ContextRepository(ctx context.Context) *repository {
	var r *repository
	r, _ = ctx.Value(RepositoryCtxtKey).(*repository)
	return r
}

func ContextAccount(ctx context.Context) *Account {
	var a *Account
	a, _ = ctx.Value(LoggedAccountCtxtKey).(*Account)
	return a
}

func ContextAuthors(ctx context.Context) []Account {
	var a []Account
	a, _ = ctx.Value(AuthorCtxtKey).([]Account)
	return a
}

func ContextCursor(ctx context.Context) *Cursor {
	var c *Cursor
	c, _ = ctx.Value(CursorCtxtKey).(*Cursor)
	return c
}

func ContextContent(ctx context.Context) *Item {
	var i *Item
	i, _ = ctx.Value(ContentCtxtKey).(*Item)
	return i
}

func ContextRegisterModel(ctx context.Context) *registerModel {
	var r *registerModel
	r, _ = ctx.Value(ModelCtxtKey).(*registerModel)
	return r
}
