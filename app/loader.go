package app

import (
	"context"
	"fmt"
	as "github.com/go-ap/activitypub"
	"github.com/mariusor/qstring"
	"net/url"
	"strings"
	"time"
)

type CtxtKey string

var (
	AccountCtxtKey    CtxtKey = "__acct"
	RepositoryCtxtKey CtxtKey = "__repository"
	FilterCtxtKey     CtxtKey = "__filter"

	CollectionCtxtKey      CtxtKey = "__collection"
	CollectionCountCtxtKey CtxtKey = "__collection_count"
	ItemCtxtKey            CtxtKey = "__item"
)

type MatchType int
type ItemType string
type VoteType string
type MimeType string

const (
	MatchEquals = MatchType(1 << iota)
	MatchFuzzy
	MatchBefore
	MatchAfter
)

const (
	TypeUndo    = VoteType("undo")
	TypeDislike = VoteType("dislike")
	TypeLike    = VoteType("like")
	ContextNil  = "0"
)

type Info struct {
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

type Paginator interface {
	QueryString() string
	BasePage() Paginator
	CurrentPage() Paginator
	NextPage() Paginator
	PrevPage() Paginator
	FirstPage() Paginator
	CurrentIndex() int
}

type VoteTypes []VoteType
type Hashes []Hash

func (vt VoteTypes) String() string {
	str := make([]string, len(vt))
	for i := range vt {
		str[i] = string(vt[i])
	}
	return strings.Join(str, ", ")
}

func (h Hashes) Contains(s Hash) bool {
	for _, hh  := range h {
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

type LoadVotesFilter struct {
	ItemKey              []Hash                     `qstring:"object,omitempty"`
	Type                 as.ActivityVocabularyTypes `qstring:"type,omitempty"`
	AttributedTo         []Hash                     `qstring:"attributedTo,omitempty"`
	SubmittedAt          time.Time                  `qstring:"submittedAt,omitempty"`
	SubmittedAtMatchType MatchType                  `qstring:"submittedAtMatchType,omitempty"`
}

type LoadItemsFilter struct {
	Key                  []Hash     `qstring:"iri,omitempty"`
	MediaType            []MimeType `qstring:"mediaType,omitempty"`
	AttributedTo         []Hash     `qstring:"attributedTo,omitempty"`
	InReplyTo            []string   `qstring:"inReplyTo,omitempty"`
	Context              []string   `qstring:"context,omitempty"`
	SubmittedAt          time.Time  `qstring:"submittedAt,omitempty"`
	SubmittedAtMatchType MatchType  `qstring:"submittedAtMatchType,omitempty"`
	Content              string     `qstring:"content,omitempty"`
	ContentMatchType     MatchType  `qstring:"contentMatchType,omitempty"`
	Deleted              []bool     `qstring:"-"` // used as an array to allow for it to be missing
	IRI                  string     `qstring:"id,omitempty"`
	URL                  string     `qstring:"url,omitempty"`
	Depth                int        `qstring:"depth,omitempty"`
	Federated            []bool     `qstring:"-"` // used as an array to allow for it to be missing
	// FollowedBy is the hash or handle of the user of which we should show the list of items that were commented on or liked
	FollowedBy   string `qstring:"followedBy,omitempty"`
	contentAlias string
	authorAlias  string
}

type Filters struct {
	LoadAccountsFilter
	LoadItemsFilter
	LoadVotesFilter
	Page     int `qstring:"page,omitempty"`
	MaxItems int `qstring:"maxItems,omitempty"`
}

type LoadAccountsFilter struct {
	Key      []Hash   `qstring:"hash,omitempty"`
	Handle   []string `qstring:"name,omitempty"`
	Email    []string `qstring:"email,omitempty"`
	Deleted  []bool   `qstring:"deleted,omitempty"`
	IRI      string   `qstring:"id,omitempty"`
	InboxIRI string   `qstring:"inbox,omitempty"`
}

func (v VoteType) String() string {
	return strings.ToLower(string(v))
}

//func (m MimeType) String() string {
//	return url.QueryEscape(strings.ToLower(string(m)))
//}

func query(f Filterable) string {
	res := ""

	var u url.Values
	var err error
	if u, err = qstring.Marshal(f); err != nil {
		return ""
	}

	if len(u) > 0 {
		res = "?" + u.Encode()
	}
	return res
}

// @todo(marius) the GetWhereClauses methods should be moved to the db package into a different format
func (f LoadVotesFilter) GetWhereClauses() ([]string, []interface{}) {
	wheres := make([]string, 0)
	whereValues := make([]interface{}, 0)
	counter := 0
	if len(f.AttributedTo) > 0 {
		whereColumns := make([]string, 0)
		for _, v := range f.AttributedTo {
			whereColumns = append(whereColumns, fmt.Sprintf(`"voter"."key" ~* ?%d`, counter))
			whereValues = append(whereValues, interface{}(v))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR ")))
	}
	if len(f.Type) > 0 {
		whereColumns := make([]string, 0)
		for _, typ := range f.Type {
			switch strings.ToLower(string(typ)) {
			case string(TypeLike):
				whereColumns = append(whereColumns, fmt.Sprintf(`"vote"."weight" > ?%d`, counter))
			case string(TypeDislike):
				whereColumns = append(whereColumns, fmt.Sprintf(`"vote"."weight" < ?%d`, counter))
			case string(TypeUndo):
				whereColumns = append(whereColumns, fmt.Sprintf(`"vote"."weight" = ?%d`, counter))
			}
			whereValues = append(whereValues, interface{}(0))
			counter++
		}
		if len(whereColumns) > 0 && len(whereColumns) < 3 {
			wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR ")))
		}
	}
	if len(f.ItemKey) > 0 {
		whereColumns := make([]string, 0)
		for _, k := range f.ItemKey {
			h := trimHash(k)
			if len(h) == 0 {
				continue
			}
			whereColumns = append(whereColumns, fmt.Sprintf(`"item"."key" ~* ?%d`, counter))
			whereValues = append(whereValues, interface{}(h))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR ")))
	}
	return wheres, whereValues
}

func copyVotesFilters(a *LoadVotesFilter, b LoadVotesFilter) {
	a.ItemKey = b.ItemKey
	a.Type = b.Type
	a.AttributedTo = b.AttributedTo
	a.SubmittedAt = b.SubmittedAt
	a.SubmittedAtMatchType = b.SubmittedAtMatchType
}

// @todo(marius) the WithContentAlias methods should be moved to the db package into a different format
//   as we can see here, the aliases show that
func (f *LoadItemsFilter) WithAuthorAlias(s string) *LoadItemsFilter {
	f.authorAlias = s
	return f
}

// @todo(marius) the WithContentAlias methods should be moved to the db package into a different format
//   as we can see here, the aliases show that
func (f *LoadItemsFilter) WithContentAlias(s string) *LoadItemsFilter {
	f.contentAlias = s
	return f
}

// @todo(marius) the GetWhereClauses methods should be moved to the db package into a different format
func (f LoadItemsFilter) GetWhereClauses() ([]string, []interface{}) {
	wheres := make([]string, 0)
	whereValues := make([]interface{}, 0)

	it := f.contentAlias
	acc := f.authorAlias
	counter := 0
	if len(f.Key) > 0 {
		keyWhere := make([]string, 0)
		for _, hash := range f.Key {
			keyWhere = append(keyWhere, fmt.Sprintf(`"%s"."key" ~* ?%d`, it, counter))
			whereValues = append(whereValues, interface{}(hash))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(keyWhere, " OR ")))
	}
	if len(f.AttributedTo) > 0 {
		attrWhere := make([]string, 0)
		for _, v := range f.AttributedTo {
			attrWhere = append(attrWhere, fmt.Sprintf(`"%s"."key" ~* ?%d`, acc, counter))
			attrWhere = append(attrWhere, fmt.Sprintf(`"%s"."handle" = ?%d`, acc, counter))
			whereValues = append(whereValues, interface{}(v))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(attrWhere, " OR ")))
	}
	if len(f.Context) > 0 {
		// Context filters are hashes belonging to a top element
		ctxtWhere := make([]string, 0)
		for _, ctxtHash := range f.Context {
			if ctxtHash == ContextNil || ctxtHash == "" {
				ctxtWhere = append(ctxtWhere, fmt.Sprintf(`"%s"."path" is NULL OR nlevel("%s"."path") = 0`, it, it))
				break
			}
			ctxtWhere = append(ctxtWhere, fmt.Sprintf(`("%s"."path" <@ (SELECT
CASE WHEN "path" IS NULL THEN "key"::ltree ELSE ltree_addltree("path", "key"::ltree) END
FROM "items" WHERE "key" ~* ?%d) AND "%s"."path" IS NOT NULL)`, it, counter, it))
			if f.Depth > 0 {
				wheres = append(wheres, fmt.Sprintf(`nlevel("%s"."path") <= (SELECT CASE WHEN "path" IS NULL THEN 0 ELSE nlevel("path")::INT END FROM "items" WHERE "key" ~* ?%d) + %d`, it, counter, f.Depth))
				f.Depth = 0
			}
			whereValues = append(whereValues, interface{}(ctxtHash))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(ctxtWhere, " OR ")))
	}
	if len(f.InReplyTo) > 0 {
		whereColumns := make([]string, 0)
		for _, hash := range f.InReplyTo {
			if len(hash) == 0 {
				continue
			}
			whereColumns = append(whereColumns, fmt.Sprintf(`("%s"."path" = (SELECT
CASE WHEN "path" IS NULL THEN "key"::ltree ELSE ltree_addltree("path", "key"::ltree) END
FROM "items" WHERE "key" ~* ?%d) AND "%s"."path" IS NOT NULL)`, it, counter, it))
			if f.Depth > 0 {
				wheres = append(wheres, fmt.Sprintf(`nlevel("%s"."path") <= (SELECT CASE WHEN "path" IS NULL THEN 0 ELSE nlevel("path")::INT END FROM "items" WHERE "key" ~* ?%d) + %d`, it, counter, f.Depth))
				f.Depth = 0
			}
			whereValues = append(whereValues, interface{}(hash))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR ")))
	}
	if len(f.Content) > 0 {
		contentWhere := make([]string, 0)
		var operator string
		if f.ContentMatchType == MatchFuzzy {
			operator = "~"
		}
		if f.ContentMatchType == MatchEquals {
			operator = "="
		}
		contentWhere = append(contentWhere, fmt.Sprintf(`"%s"."title" %s ?%d`, it, operator, counter))
		whereValues = append(whereValues, interface{}(f.Content))
		counter++
		contentWhere = append(contentWhere, fmt.Sprintf(`"%s"."data" %s ?%d`, it, operator, counter))
		whereValues = append(whereValues, interface{}(f.Content))
		counter++
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(contentWhere, " OR ")))
	}
	if len(f.MediaType) > 0 {
		mediaWhere := make([]string, 0)
		for _, v := range f.MediaType {
			mediaWhere = append(mediaWhere, fmt.Sprintf(`"%s"."mime_type" = ?%d`, it, counter))
			whereValues = append(whereValues, interface{}(v))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(mediaWhere, " OR ")))
	}
	if len(f.Deleted) > 0 {
		delWhere := make([]string, 0)
		for _, del := range f.Deleted {
			var eqOp string
			if del {
				eqOp = "="
			} else {
				eqOp = "!="
			}
			delWhere = append(delWhere, fmt.Sprintf(`"%s"."flags" & 1::bit(8) %s 1::bit(8)`, it, eqOp))
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(delWhere, " OR ")))
	}
	if len(f.FollowedBy) > 0 {
		keyWhere := make([]string, 0)
		hash := f.FollowedBy
		keyWhere = append(keyWhere, fmt.Sprintf(`"%s"."id" in (SELECT "votes"."item_id" FROM "votes" WHERE "votes"."submitted_by" = (SELECT "id" FROM "accounts" where "key" ~* ?%d OR "handle" = ?%d) AND "votes"."weight" != 0)
			OR
"%s"."key" IN (SELECT subpath("path", 0, 1)::varchar FROM "items" WHERE "submitted_by" = (SELECT "id" FROM "accounts" where "key" ~* ?%d OR "handle" = ?%d) AND nlevel("path") > 1)`, it, counter, counter, it, counter, counter))
		whereValues = append(whereValues, interface{}(hash))
		counter++
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(keyWhere, " OR ")))
	}
	if len(f.Federated) > 0 && len(f.Federated) < 2 {
		for _, fed := range f.Federated {
			fWheres := make([]string, 0)
			if fed {
				// TODO(marius) "attributedTo" should be more than not null,
				//              it shouldn't contain the current instance's base URL
				fWheres = append(fWheres, fmt.Sprintf(`"%s"."metadata"->>'attributedTo' IS NOT NULL`, it))
			} else {
				fWheres = append(fWheres, fmt.Sprintf(`"%s"."metadata"->>'attributedTo' IS NULL`, it))
			}
			wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(fWheres, " OR ")))
		}
	}
	if len(f.IRI) > 0 {
		wheres = append(wheres, fmt.Sprintf(`"%s"."metadata"->>'id' ~* ?%d`, it, counter))
		whereValues = append(whereValues, interface{}(f.IRI))
		counter++
	}
	if f.Depth > 0 {
		wheres = append(wheres, fmt.Sprintf(`nlevel("%s"."path") <= ?%d`, it, counter))
		whereValues = append(whereValues, interface{}(f.Depth))
	}
	return wheres, whereValues
}

func copyItemsFilters(a *LoadItemsFilter, b LoadItemsFilter) {
	a.Key = b.Key
	a.MediaType = b.MediaType
	a.AttributedTo = b.AttributedTo
	a.InReplyTo = b.InReplyTo
	a.Context = b.Context
	a.SubmittedAt = b.SubmittedAt
	a.SubmittedAtMatchType = b.SubmittedAtMatchType
	a.Content = b.Content
	a.ContentMatchType = b.ContentMatchType
	a.Deleted = b.Deleted
	a.IRI = b.IRI
	a.Deleted = b.Deleted
	a.FollowedBy = b.FollowedBy
	a.contentAlias = b.contentAlias
	a.authorAlias = b.authorAlias
}

func (f *Filters) QueryString() string {
	return query(f)
}

func (f *Filters) BasePage() Paginator {
	b := &Filters{}
	copyFilters(b, *f)
	return b
}

func (f *Filters) CurrentPage() Paginator {
	return f
}
func (f *Filters) NextPage() Paginator {
	b := &Filters{}
	copyFilters(b, *f)
	b.Page += 1
	return b
}
func (f *Filters) PrevPage() Paginator {
	b := &Filters{}
	copyFilters(b, *f)
	b.Page -= 1
	return b
}
func (f *Filters) FirstPage() Paginator {
	b := &Filters{}
	copyFilters(b, *f)
	b.Page = 1
	return b
}
func (f *Filters) CurrentIndex() int {
	return f.Page
}

// @TODO(marius) the GetLimit methods should be moved to the db package into a different format
func (f Filters) GetLimit() string {
	if f.MaxItems == 0 {
		return ""
	}
	limit := fmt.Sprintf("  LIMIT %d", f.MaxItems)
	if f.Page > 1 {
		limit = fmt.Sprintf("%s OFFSET %d", limit, f.MaxItems*(f.Page-1))
	}
	return limit
}

// @TODO(marius) the GetWhereClauses methods should be moved to the db package into a different format
func (f Filters) GetWhereClauses() ([]string, []interface{}) {
	var clauses []string
	var values []interface{}

	iCl, iVal := f.LoadItemsFilter.GetWhereClauses()
	clauses = append(clauses, iCl...)
	values = append(values, iVal...)
	aCl, aVal := f.LoadAccountsFilter.GetWhereClauses()
	clauses = append(clauses, aCl...)
	values = append(values, aVal...)
	vCl, vVal := f.LoadVotesFilter.GetWhereClauses()
	clauses = append(clauses, vCl...)
	values = append(values, vVal...)
	return clauses, values
}

// @TODO(marius) the GetWhereClauses methods should be moved to the db package into a different format
func (f LoadAccountsFilter) GetWhereClauses() ([]string, []interface{}) {
	wheres := make([]string, 0)
	whereValues := make([]interface{}, 0)
	counter := 0

	if len(f.Key) > 0 {
		whereColumns := make([]string, 0)
		for _, hash := range f.Key {
			whereColumns = append(whereColumns, fmt.Sprintf(`"accounts"."key" ~* ?%d`, counter))
			whereValues = append(whereValues, interface{}(hash))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR ")))
	}
	if len(f.Handle) > 0 {
		whereColumns := make([]string, 0)
		for _, handle := range f.Handle {
			whereColumns = append(whereColumns, fmt.Sprintf(`"accounts"."handle" = ?%d`, counter))
			whereValues = append(whereValues, interface{}(handle))
			counter++
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR ")))
	}
	if len(f.Email) > 0 {
		wheres = append(wheres, fmt.Sprintf(`"accounts"."email"  ~* ?%d`, counter))
		whereValues = append(whereValues, interface{}(f.Email))
		counter++
	}
	if len(f.InboxIRI) > 0 {
		wheres = append(wheres, fmt.Sprintf(`"accounts"."metadata"->>'inbox' ~* ?%d`, counter))
		whereValues = append(whereValues, interface{}(f.InboxIRI))
		counter++
	}
	if len(f.IRI) > 0 {
		wheres = append(wheres, fmt.Sprintf(`"accounts"."metadata"->>'id' ~* ?%d`, counter))
		whereValues = append(whereValues, interface{}(f.IRI))
		counter++
	}
	if len(f.Deleted) > 0 {
		delWhere := make([]string, 0)
		for _, del := range f.Deleted {
			var eqOp string
			if del {
				eqOp = "="
			} else {
				eqOp = "!="
			}
			delWhere = append(delWhere, fmt.Sprintf(`"flags" & 1::bit(8) %s 1::bit(8)`, eqOp))
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(delWhere, " OR ")))
	}

	return wheres, whereValues
}

func copyFilters(a *Filters, b Filters) {
	copyAccountFilters(&a.LoadAccountsFilter, b.LoadAccountsFilter)
	copyItemsFilters(&a.LoadItemsFilter, b.LoadItemsFilter)
	copyVotesFilters(&a.LoadVotesFilter, b.LoadVotesFilter)
	a.MaxItems = b.MaxItems
	a.Page = b.Page
}

func copyAccountFilters(a *LoadAccountsFilter, b LoadAccountsFilter) {
	a.Key = b.Key
	a.Handle = b.Handle
	a.Email = b.Email
	a.Deleted = b.Deleted
	a.InboxIRI = b.InboxIRI
	a.IRI = b.IRI
}

type Repository interface {
	CanLoadAccounts
	CanLoadItems
	CanLoadInfo
	CanLoadVotes
}

type Authenticated interface {
	WithAccount(a *Account) error
}

type CanSaveItems interface {
	SaveItem(it Item) (Item, error)
}

type CanLoadItems interface {
	LoadItem(f Filters) (Item, error)
	LoadItems(f Filters) (ItemCollection, uint, error)
}

type CanLoadVotes interface {
	LoadVotes(f Filters) (VoteCollection, uint, error)
	LoadVote(f Filters) (Vote, error)
}

type CanLoadInfo interface {
	LoadInfo() (Info, error)
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
	//   items and accounts tables, corresponding ES documents, etc
	SaveVote(v Vote) (Vote, error)
}

type CanLoadAccounts interface {
	LoadAccount(f Filters) (Account, error)
	LoadAccounts(f Filters) (AccountCollection, uint, error)
}

type CanSaveAccounts interface {
	SaveAccount(a Account) (Account, error)
}

type CanSaveActivity interface {
	SaveActivity(as.Item, as.IRI) (as.Item, error)
}

type CanLoad interface {
	CanLoadItems
	CanLoadAccounts
	CanLoadVotes
}

type CanSave interface {
	CanSaveItems
	CanSaveAccounts
	CanSaveVotes
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

func ContextAuthenticated(ctx context.Context) (Authenticated, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	a, ok := ctxVal.(Authenticated)
	return a, ok
}

func ContextLoader(ctx context.Context) (CanLoad, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	l, ok := ctxVal.(CanLoad)
	return l, ok
}

func ContextSaver(ctx context.Context) (CanSave, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	s, ok := ctxVal.(CanSave)
	return s, ok
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

func ContextNodeInfoLoader(ctx context.Context) (CanLoadInfo, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	a, ok := ctxVal.(CanLoadInfo)
	return a, ok
}

func ContextAccount(ctx context.Context) (*Account, bool) {
	ctxVal := ctx.Value(AccountCtxtKey)
	a, ok := ctxVal.(*Account)
	return a, ok
}

func ContextActivitySaver(ctx context.Context) (CanSaveActivity, bool) {
	ctxVal := ctx.Value(RepositoryCtxtKey)
	a, ok := ctxVal.(CanSaveActivity)
	return a, ok
}
