package models

import (
	"database/sql"
	"net/http"
	"golang.org/x/net/context"
	"github.com/juju/errors"
	"time"
	)

var Db *sql.DB

const ServiceCtxtKey = "__loader"
// Loader middleware
func Loader (next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		newCtx := context.WithValue(ctx, ServiceCtxtKey, Service)
		next.ServeHTTP(w, r.WithContext(newCtx))
	}
	return http.HandlerFunc(fn)
}

type MatchType string
type ItemType string
type VoteType string

const (
	MatchEquals = MatchType(1 << iota)
	MatchBefore
	MatchAfter
	MatchFuzzy
)

const (
	TypeOP = ItemType("op")

	TypeLike = VoteType("like")
	TypeDislike = VoteType("dislike")
)

type LoadVotesFilter struct {
	ItemKey []string
	Type []VoteType
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
	Parent []string
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

type LoadAccountFilter struct {
	Key string
	Handle string
}

type CanSaveItems interface {
	SaveItem(it Item) (Item, error)
}

type CanLoadItems interface {
	LoadItem(f LoadItemFilter) (Item, error)
	LoadItems(f LoadItemsFilter) (ItemCollection, error)
}

type CanLoadVotes interface {
	LoadVotes(f LoadVotesFilter) (VoteCollection, error)
	LoadVote(f LoadVotesFilter) (Vote, error)
}

type CanSaveVotes interface {
	SaveVote(v Vote) (Vote, error)
}

type CanLoadAccounts interface {
	LoadAccount(f LoadAccountFilter) (Account, error)
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

func (l LoaderService) SaveVote(v Vote) (Vote, error) {
	return SaveVote(v)
}

func (l LoaderService) LoadVotes(f LoadVotesFilter) (VoteCollection, error) {
	return LoadVotes(f)
}

func (l LoaderService) LoadVote(f LoadVotesFilter) (Vote, error) {
	f.MaxItems = 1
	votes, err := LoadVotes(f)
	if err != nil {
		return Vote{}, err
	}
	for _, vote := range votes {
		return vote, nil
	}
	return Vote{}, errors.Errorf("not found")
}

func (l LoaderService) LoadAccount(f LoadAccountFilter) (Account, error) {
	return loadAccount(f.Handle)
}
