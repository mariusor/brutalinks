package models

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context"
)

const RepositoryCtxtKey = "__repository"

// Repository middleware
func Repository(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		newCtx := context.WithValue(ctx, RepositoryCtxtKey, Config)
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
	TypeDislike = VoteType("dislike")
	TypeLike    = VoteType("like")
	ContextNil  = "0"
)

type LoadVotesFilter struct {
	ItemKey              []string   `qstring:"hash,omitempty"`
	Type                 []VoteType `qstring:"type,omitempty"`
	AttributedTo         []Hash     `qstring:"attributedTo,omitempty"`
	SubmittedAt          time.Time  `qstring:"submittedAt,omitempty"`
	SubmittedAtMatchType MatchType  `qstring:"submittedAtMatchType,omitempty"`
	Page                 int        `qstring:"page,omitempty"`
	MaxItems             int        `qstring:"maxItems,omitempty"`
}

type LoadItemsFilter struct {
	Key                  []string  `qstring:"hash,omitempty"`
	MediaType            []string  `qstring:"mediaType,omitempty"`
	AttributedTo         []Hash    `qstring:"attributedTo,omitempty"`
	InReplyTo            []string  `qstring:"inReplyTo,omitempty"`
	Context              []string  `qstring:"context,omitempty"`
	SubmittedAt          time.Time `qstring:"submittedAt,omitempty"`
	SubmittedAtMatchType MatchType `qstring:"submittedAtMatchType,omitempty"`
	Content              string    `qstring:"content,omitempty"`
	ContentMatchType     MatchType `qstring:"contentMatchType,omitempty"`
	Deleted              bool      `qstring:"deleted,omitempty"`
	Page                 int       `qstring:"page,omitempty"`
	MaxItems             int       `qstring:"maxItems,omitempty"`
}

type LoadAccountsFilter struct {
	Key      []string `qstring:"hash,omitempty"`
	Handle   []string `qstring:"handle,omitempty"`
	Deleted  bool     `qstring:"deleted,omitempty"`
	Page     int      `qstring:"page,omitempty"`
	MaxItems int      `qstring:"maxItems,omitempty"`
}

func (f LoadVotesFilter) GetWhereClauses() ([]string, []interface{}) {
	wheres := make([]string, 0)
	whereValues := make([]interface{}, 0)
	counter := 1
	if len(f.AttributedTo) > 0 {
		whereColumns := make([]string, 0)
		for _, v := range f.AttributedTo {
			whereColumns = append(whereColumns, fmt.Sprintf(`"voter"."key" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(v))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR ")))
	}
	if len(f.Type) > 0 {
		whereColumns := make([]string, 0)
		for _, typ := range f.Type {
			if typ == TypeLike {
				whereColumns = append(whereColumns, fmt.Sprintf(`"votes"."weight" > $%d`, counter))
				whereValues = append(whereValues, interface{}(0))
			}
			if typ == TypeDislike {
				whereColumns = append(whereColumns, fmt.Sprintf(`"votes"."weight" < $%d`, counter))
				whereValues = append(whereValues, interface{}(0))
			}
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	if len(f.ItemKey) > 0 {
		whereColumns := make([]string, 0)
		for _, k := range f.ItemKey {
			h := trimHash(k)
			if len(h) == 0 {
				continue
			}
			whereColumns = append(whereColumns, fmt.Sprintf(`"items"."key" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(h))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	return wheres, whereValues
}

func (filter LoadItemsFilter) GetWhereClauses() ([]string, []interface{}) {
	wheres := make([]string, 0)
	whereValues := make([]interface{}, 0)
	counter := 1
	if len(filter.Key) > 0 {
		keyWhere := make([]string, 0)
		for _, hash := range filter.Key {
			keyWhere = append(keyWhere, fmt.Sprintf(`"content_items"."key" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(hash))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(keyWhere, " OR "))))
	}
	if len(filter.AttributedTo) > 0 {
		attrWhere := make([]string, 0)
		for _, v := range filter.AttributedTo {
			attrWhere = append(attrWhere, fmt.Sprintf(`"accounts"."key" ~* $%d`, counter))
			attrWhere = append(attrWhere, fmt.Sprintf(`"accounts"."handle" = $%d`, counter))
			whereValues = append(whereValues, interface{}(v))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(attrWhere, " OR ")))
	}
	if len(filter.Context) > 0 {
		// Context filters are hashes belonging to a top element
		ctxtWhere := make([]string, 0)
		for _, ctxtHash := range filter.Context {
			if ctxtHash == ContextNil || ctxtHash == "" {
				ctxtWhere = append(ctxtWhere, `"content_items"."path" is NULL OR nlevel("content_items"."path") = 0`)
				break
			}
			ctxtWhere = append(ctxtWhere, fmt.Sprintf(`("content_items"."path" <@ (select
CASE WHEN path is null THEN key::ltree ELSE ltree_addltree(path, key::ltree) END
from "content_items" where key ~* $%d) AND "content_items"."path" IS NOT NULL)`, counter))
			whereValues = append(whereValues, interface{}(ctxtHash))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(ctxtWhere, " OR "))))
	}
	if len(filter.InReplyTo) > 0 {
		whereColumns := make([]string, 0)
		for _, hash := range filter.InReplyTo {
			if len(hash) == 0 {
				continue
			}
			whereColumns = append(whereColumns, fmt.Sprintf(`("content_items"."path" <@ (select
CASE WHEN path is null THEN key::ltree ELSE ltree_addltree(path, key::ltree) END
from "content_items" where key ~* $%d) AND "content_items"."path" IS NOT NULL)`, counter))
			whereValues = append(whereValues, interface{}(hash))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	if len(filter.Content) > 0 {
		contentWhere := make([]string, 0)
		var operator string
		if filter.ContentMatchType == MatchFuzzy {
			operator = "~"
		}
		if filter.ContentMatchType == MatchEquals {
			operator = "="
		}
		contentWhere = append(contentWhere, fmt.Sprintf(`"content_items"."title" %s $%d`, operator, counter))
		whereValues = append(whereValues, interface{}(filter.Content))
		counter += 1
		contentWhere = append(contentWhere, fmt.Sprintf(`"content_items"."data" %s $%d`, operator, counter))
		whereValues = append(whereValues, interface{}(filter.Content))
		counter += 1
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(contentWhere, " OR ")))
	}
	if len(filter.MediaType) > 0 {
		mediaWhere := make([]string, 0)
		for _, v := range filter.MediaType {
			mediaWhere = append(mediaWhere, fmt.Sprintf(`"content_items"."mime_type" = $%d`, counter))
			whereValues = append(whereValues, interface{}(v))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(mediaWhere, " OR ")))
	}
	var eqOp string
	if filter.Deleted {
		eqOp = "="
	} else {
		eqOp = "!="
	}
	whereDeleted := fmt.Sprintf(`"content_items"."flags" & $%d::bit(8) %s $%d::bit(8)`, counter, eqOp, counter)
	whereValues = append(whereValues, interface{}(FlagsDeleted))
	counter += 1
	wheres = append(wheres, fmt.Sprintf("%s", whereDeleted))

	return wheres, whereValues
}

func (f LoadAccountsFilter) GetWhereClauses() ([]string, []interface{}) {
	wheres := make([]string, 0)
	whereValues := make([]interface{}, 0)
	counter := 1

	if len(f.Key) > 0 {
		whereColumns := make([]string, 0)
		for _, hash := range f.Key {
			whereColumns = append(whereColumns, fmt.Sprintf(`"accounts"."key" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(hash))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	if len(f.Handle) > 0 {
		whereColumns := make([]string, 0)
		for _, handle := range f.Handle {
			whereColumns = append(whereColumns, fmt.Sprintf(`"accounts"."handle" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(handle))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}

	return wheres, whereValues
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
	LoadAccount(f LoadAccountsFilter) (Account, error)
	LoadAccounts(f LoadAccountsFilter) (AccountCollection, error)
}

type CanSaveAccounts interface {
	SaveAccount(a Account) (Account, error)
}

// I think we can move from using the exported Config package variable
// to an unexported one. First we need to decouple the DB config from the repository struct to a config struct
var Config repository

type repository struct {
	DB *sql.DB
}

func (l repository) SaveItem(it Item) (Item, error) {
	return saveItem(l.DB, it)
}

func (l repository) LoadItem(f LoadItemsFilter) (Item, error) {
	return loadItem(l.DB, f)
}

func (l repository) LoadItems(f LoadItemsFilter) (ItemCollection, error) {
	return loadItems(l.DB, f)
}

func (l repository) SaveVote(v Vote) (Vote, error) {
	return saveVote(l.DB, v)
}

func (l repository) LoadVotes(f LoadVotesFilter) (VoteCollection, error) {
	return loadVotes(l.DB, f)
}

func (l repository) LoadVote(f LoadVotesFilter) (Vote, error) {
	f.MaxItems = 1
	votes, err := loadVotes(l.DB, f)
	if err != nil {
		return Vote{}, err
	}
	v, err := votes.First()
	return *v, err
}

func (l repository) LoadAccount(f LoadAccountsFilter) (Account, error) {
	return loadAccount(l.DB, f)
}

func (l repository) LoadAccounts(f LoadAccountsFilter) (AccountCollection, error) {
	return loadAccounts(l.DB, f)
}

func (l repository) SaveAccount(a Account) (Account, error) {
	return saveAccount(l.DB, a)
}
