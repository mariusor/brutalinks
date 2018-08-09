package models

import (
	"database/sql"
		"net/http"
	"golang.org/x/net/context"
	"time"
	)

var Db *sql.DB

// Loader middleware
func Loader (next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		newCtx := context.WithValue(ctx, "loader", Service)
		next.ServeHTTP(w, r.WithContext(newCtx))
	}
	return http.HandlerFunc(fn)
}

type MatchType string
type ItemType string

const (
	MatchEquals = MatchType(1 << iota)
	MatchBefore
	MatchAfter
	MatchFuzzy
)

const (
	TypeOP = ItemType("op")
)

type LoadVotesFilter struct {
	ItemKey []string
	Type string
	SubmittedBy []string
	SubmittedAt time.Time
	SubmittedAtMatchType MatchType
	Page int
	MaxItems int
}

type LoadItemsFilter struct {
	Key []string
	Type []ItemType
	MediaType []string
	SubmittedBy []string
	Parent string
	OP string
	SubmittedAt time.Time
	SubmittedAtMatchType MatchType
	Content string
	ContentMatchType MatchType
	Page int
	MaxItems int
}

type LoadItemFilter struct {
	Key string
}

type CanLoadItems interface {
	LoadItem(f LoadItemFilter) (Item, error)
	LoadItems(f LoadItemsFilter) (ItemCollection, error)
}

type CanLoadVotes interface {
	LoadVotes(f LoadVotesFilter) (VoteCollection, error)
}

type CanSaveItems interface {
	SaveItem(it Item) (Item, error)
}

var Service LoaderService

type LoaderService struct {
	DB *sql.DB
}

func (l LoaderService) SaveItem(it Item) (Item, error) {
	return SaveItem(it)
}

func (l LoaderService) LoadItem(f LoadItemFilter) (Item, error) {
	return LoadItem(f)
}

func (l LoaderService) LoadItems(f LoadItemsFilter) (ItemCollection, error) {
	return LoadItems(f)
}

func (l LoaderService) LoadVotes(f LoadVotesFilter) (VoteCollection, error) {
	return LoadVotes(f)
}

