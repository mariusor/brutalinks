package models

import (
	"context"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

var Logger log.FieldLogger

const (
	AccountCtxtKey    = "__acct"
	RepositoryCtxtKey = "__repository"
	FilterCtxtKey     = "__filter"
)

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

func init() {
	if Logger == nil {
		Logger = log.StandardLogger()
	}
}

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
	Deleted              []bool    `qstring:"deleted,omitempty"`
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
			counter++
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
			counter++
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
			whereColumns = append(whereColumns, fmt.Sprintf(`"item"."key" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(h))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	return wheres, whereValues
}

func (filter LoadItemsFilter) GetWhereClauses(it string, acc string) ([]string, []interface{}) {
	wheres := make([]string, 0)
	whereValues := make([]interface{}, 0)
	counter := 1
	if len(filter.Key) > 0 {
		keyWhere := make([]string, 0)
		for _, hash := range filter.Key {
			keyWhere = append(keyWhere, fmt.Sprintf(`"%s"."key" ~* $%d`, it, counter))
			whereValues = append(whereValues, interface{}(hash))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(keyWhere, " OR "))))
	}
	if len(filter.AttributedTo) > 0 {
		attrWhere := make([]string, 0)
		for _, v := range filter.AttributedTo {
			attrWhere = append(attrWhere, fmt.Sprintf(`"%s"."key" ~* $%d`, acc, counter))
			attrWhere = append(attrWhere, fmt.Sprintf(`"%s"."handle" = $%d`, acc, counter))
			whereValues = append(whereValues, interface{}(v))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(attrWhere, " OR ")))
	}
	if len(filter.Context) > 0 {
		// Context filters are hashes belonging to a top element
		ctxtWhere := make([]string, 0)
		for _, ctxtHash := range filter.Context {
			if ctxtHash == ContextNil || ctxtHash == "" {
				ctxtWhere = append(ctxtWhere, fmt.Sprintf(`"%s"."path" is NULL OR nlevel("%s"."path") = 0`, it, it))
				break
			}
			ctxtWhere = append(ctxtWhere, fmt.Sprintf(`("%s"."path" <@ (SELECT
CASE WHEN "path" IS NULL THEN "key"::ltree ELSE ltree_addltree("path", "key"::ltree) END
FROM "content_items" WHERE "key" ~* $%d) AND "%s"."path" IS NOT NULL)`, it, counter, it))
			whereValues = append(whereValues, interface{}(ctxtHash))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(ctxtWhere, " OR "))))
	}
	if len(filter.InReplyTo) > 0 {
		whereColumns := make([]string, 0)
		for _, hash := range filter.InReplyTo {
			if len(hash) == 0 {
				continue
			}
			whereColumns = append(whereColumns, fmt.Sprintf(`("%s"."path" <@ (SELECT
CASE WHEN "path" IS NULL THEN "key"::ltree ELSE ltree_addltree("path", "key"::ltree) END
FROM "content_items" WHERE "key" ~* $%d) AND "%s"."path" IS NOT NULL)`, it, counter, it))
			whereValues = append(whereValues, interface{}(hash))
			counter++
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
		contentWhere = append(contentWhere, fmt.Sprintf(`"%s"."title" %s $%d`, it, operator, counter))
		whereValues = append(whereValues, interface{}(filter.Content))
		counter++
		contentWhere = append(contentWhere, fmt.Sprintf(`"%s"."data" %s $%d`, it, operator, counter))
		whereValues = append(whereValues, interface{}(filter.Content))
		counter++
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(contentWhere, " OR ")))
	}
	if len(filter.MediaType) > 0 {
		mediaWhere := make([]string, 0)
		for _, v := range filter.MediaType {
			mediaWhere = append(mediaWhere, fmt.Sprintf(`"%s"."mime_type" = $%d`, it, counter))
			whereValues = append(whereValues, interface{}(v))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(mediaWhere, " OR ")))
	}
	if len(filter.Deleted) > 0 {
		delWhere := make([]string, 0)
		for _, del := range filter.Deleted {
			var eqOp string
			if del {
				eqOp = "="
			} else {
				eqOp = "!="
			}
			delWhere = append(delWhere, fmt.Sprintf(`"%s"."flags" & $%d::bit(8) %s $%d::bit(8)`, it, counter, eqOp, counter))
			whereValues = append(whereValues, interface{}(FlagsDeleted))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(delWhere, " OR ")))
	}
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
			counter++
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	if len(f.Handle) > 0 {
		whereColumns := make([]string, 0)
		for _, handle := range f.Handle {
			whereColumns = append(whereColumns, fmt.Sprintf(`"accounts"."handle" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(handle))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}

	return wheres, whereValues
}

type Authenticated interface {
	WithAccount(a *Account)
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
	// SaveVote adds a vote to the p content item
	//   const {
	//      add_vote = "add_vote"
	//      delete = "delete"
	//   }
	//   type queue_message struct {
	//       type    string
	//       payload json.RawMessage
	//   }
	// Ideally this should be done asynchronously pushing an add_vote message to our
	// messaging queue. Details of this queue to be established (strongest possibility is Redis PubSub)
	// The cli/votes/main.go script would be responsible with waiting on the queue for these messages
	// and updating the new score and all models dependent on it.
	//   content_items and accounts tables, corresponding ES documents, etc
	SaveVote(v Vote) (Vote, error)
}

type CanLoadAccounts interface {
	LoadAccount(f LoadAccountsFilter) (Account, error)
	LoadAccounts(f LoadAccountsFilter) (AccountCollection, error)
}

type CanSaveAccounts interface {
	SaveAccount(a Account) (Account, error)
}

func ContextVoteLoader(ctx context.Context) (CanLoadVotes, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	l, ok := ctxVal.(CanLoadVotes)
	return l, ok
}

func ContextItemLoader(ctx context.Context) (CanLoadItems, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	l, ok := ctxVal.(CanLoadItems)
	return l, ok
}

func ContextCurrentAccount(ctx context.Context) (*Account, bool) {
	ctxVal := ctx.Value(AccountCtxtKey)
	if a, ok := ctxVal.(*Account); ok {
		Logger.WithFields(log.Fields{
			"handle": a.Handle,
			"hash":   a.Hash,
		}).Debugf("loaded account from context")
		return a, true
	}
	return nil, false
}

func ContextAuthenticated(ctx context.Context) (Authenticated, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	a, ok := ctxVal.(Authenticated)
	return a, ok
}

func ContextAccountLoader(ctx context.Context) (CanLoadAccounts, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	l, ok := ctxVal.(CanLoadAccounts)
	return l, ok
}

func ContextItemSaver(ctx context.Context) (CanSaveItems, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	s, ok := ctxVal.(CanSaveItems)
	return s, ok
}

func ContextAccountSaver(ctx context.Context) (CanSaveAccounts, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	s, ok := ctxVal.(CanSaveAccounts)
	return s, ok
}

func ContextVoteSaver(ctx context.Context) (CanSaveVotes, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	s, ok := ctxVal.(CanSaveVotes)
	return s, ok
}
