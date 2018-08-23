package models

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/juju/errors"
	"golang.org/x/net/context"
)

const ServiceCtxtKey = "__loader"

// Loader middleware
func Loader(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		newCtx := context.WithValue(ctx, ServiceCtxtKey, Service)
		next.ServeHTTP(w, r.WithContext(newCtx))
	}
	return http.HandlerFunc(fn)
}

type MatchType int
type ItemType string
type VoteType string

const (
	MatchEquals = MatchType(1 << iota)
	MatchFuzzy
	MatchBefore
	MatchAfter
)

const (
	TypeLike    = VoteType("like")
	TypeDislike = VoteType("dislike")
	ContextNil  = "0"
)

type LoadVotesFilter struct {
	ItemKey              []string
	Type                 []VoteType
	AttributedTo         []string
	SubmittedAt          time.Time
	SubmittedAtMatchType MatchType
	Page                 int
	MaxItems             int
}

type LoadItemsFilter struct {
	Key                  []string
	MediaType            []string
	AttributedTo         []string
	InReplyTo            []string
	Context              []string
	SubmittedAt          time.Time
	SubmittedAtMatchType MatchType
	Content              string
	ContentMatchType     MatchType
	Page                 int
	MaxItems             int
}

type LoadAccountFilter struct {
	Key    string
	Handle string
}

type CanSaveItems interface {
	SaveItem(it Item) (Item, error)
}

type CanLoadItems interface {
	LoadItem(f LoadItemsFilter) (Item, error)
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
	return saveItem(l.DB, it)
}

func (l LoaderService) LoadItem(f LoadItemsFilter) (Item, error) {
	return loadItem(l.DB, f)
}

func (l LoaderService) LoadItems(f LoadItemsFilter) (ItemCollection, error) {
	return loadItems(l.DB, f)
}

func (l LoaderService) SaveVote(v Vote) (Vote, error) {
	return saveVote(l.DB, v)
}

func (l LoaderService) LoadVotes(f LoadVotesFilter) (VoteCollection, error) {
	return loadVotes(l.DB, f)
}

func (l LoaderService) LoadVote(f LoadVotesFilter) (Vote, error) {
	f.MaxItems = 1
	votes, err := loadVotes(l.DB, f)
	if err != nil {
		return Vote{}, err
	}
	for _, vote := range votes {
		return vote, nil
	}
	return Vote{}, errors.Errorf("not found")
}

func (l LoaderService) LoadAccount(f LoadAccountFilter) (Account, error) {
	return loadAccount(l.DB, f.Handle)
}
