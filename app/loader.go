package app

import (
	"context"
	"fmt"
	"strings"
)

type CtxtKey string

var (
	AccountCtxtKey    CtxtKey = "__acct"
	RepositoryCtxtKey CtxtKey = "__repository"
	FilterCtxtKey     CtxtKey = "__filter"
	ModelCtxtKey     CtxtKey = "__model"
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
	TypeUndo    = VoteType("undo")
	TypeDislike = VoteType("dislike")
	TypeLike    = VoteType("like")
	ContextNil  = "0"
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

func (v VoteType) String() string {
	return strings.ToLower(string(v))
}

//func (m MimeType) String() string {
//	return url.QueryEscape(strings.ToLower(string(m)))
//}

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

func (f *Filters) QueryString() string {
	return query(f)
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
	ctxVal := ctx.Value(AccountCtxtKey)
	if a, ok := ctxVal.(*Account); ok {
		return a
	}
	return nil
}
