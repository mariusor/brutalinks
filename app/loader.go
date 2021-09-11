package app

import (
	"context"
)

type CtxtKey string

var (
	ServicesCtxtKey      CtxtKey = "__di"
	LoggedAccountCtxtKey CtxtKey = "__acct"
	RepositoryCtxtKey    CtxtKey = "__repository"
	FilterCtxtKey        CtxtKey = "__filter"
	LoadsCtxtKey         CtxtKey = "__loads"
	ModelCtxtKey         CtxtKey = "__model"
	AuthorCtxtKey        CtxtKey = "__author"
	CursorCtxtKey        CtxtKey = "__cursor"
	ContentCtxtKey       CtxtKey = "__content"
	DependenciesCtxtKey  CtxtKey = "__deps"
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

func ContextModerationModel(ctx context.Context) *moderationModel {
	var m *moderationModel
	m, _ = ctx.Value(ModelCtxtKey).(*moderationModel)
	return m
}

func ContextRepository(ctx context.Context) *repository {
	if r, ok := ctx.Value(RepositoryCtxtKey).(*repository); ok {
		return r
	}
	return nil
}

func ContextAccount(ctx context.Context) *Account {
	if a, ok := ctx.Value(LoggedAccountCtxtKey).(*Account); ok {
		return a
	}
	return nil
}

func ContextAuthors(ctx context.Context) AccountCollection {
	if a, ok := ctx.Value(AuthorCtxtKey).(AccountCollection); ok {
		return a
	}
	return nil
}

func ContextCursor(ctx context.Context) *Cursor {
	if c, ok := ctx.Value(CursorCtxtKey).(*Cursor); ok {
		return c
	}
	return nil
}

func ContextItem(ctx context.Context) *Item {
	if i, ok := ctx.Value(ContentCtxtKey).(*Item); ok {
		return i
	}
	if c := ContextCursor(ctx); c != nil {
		for _, it := range c.items {
			if i, ok := it.(*Item); ok {
				return i
			}
		}
	}
	return nil
}

func ContextRegisterModel(ctx context.Context) *registerModel {
	if r, ok := ctx.Value(ModelCtxtKey).(*registerModel); ok {
		return r
	}
	return nil
}

func ContextDependentLoads(ctx context.Context) *deps {
	if r, ok := ctx.Value(DependenciesCtxtKey).(*deps); ok {
		return r
	}
	return &deps{}
}
