package app

import (
	"context"
	"strings"
)

type CtxtKey string

var (
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
	if l, ok := ctx.Value(ModelCtxtKey).(Model); ok {
		return l
	}
	return nil
}

func ContextListingModel(ctx context.Context) *listingModel {
	if l, ok := ctx.Value(ModelCtxtKey).(*listingModel); ok {
		return l
	}
	return nil
}

func ContextContentModel(ctx context.Context) *contentModel {
	if l, ok := ctx.Value(ModelCtxtKey).(*contentModel); ok {
		return l
	}
	return nil
}

func ContextRepository(ctx context.Context) *repository {
	if l, ok := ctx.Value(RepositoryCtxtKey).(*repository); ok {
		return l
	}
	return nil
}

func ContextAccount(ctx context.Context) *Account {
	ctxVal := ctx.Value(LoggedAccountCtxtKey)
	if a, ok := ctxVal.(*Account); ok {
		return a
	}
	return nil
}

func ContextAuthors(ctx context.Context) []Account {
	ctxVal := ctx.Value(AuthorCtxtKey)
	if a, ok := ctxVal.([]Account); ok {
		return a
	}
	return nil
}

func ContextCursor(ctx context.Context) *Cursor {
	ctxVal := ctx.Value(CursorCtxtKey)
	if c, ok := ctxVal.(*Cursor); ok {
		return c
	}
	return nil
}

func ContextContent(ctx context.Context) *Item {
	ctxVal := ctx.Value(ContentCtxtKey)
	if i, ok := ctxVal.(*Item); ok {
		return i
	}
	return nil
}

func ContextRegisterModel(ctx context.Context) *registerModel {
	if r, ok := ctx.Value(ModelCtxtKey).(*registerModel); ok {
		return r
	}
	return nil
}
