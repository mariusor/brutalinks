package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/mariusor/littr.go/app"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"
)

type Item struct {
	ID          int64     `db:"id,auto"`
	Key         app.Key   `db:"key,size(32)"`
	Title       []byte    `db:"title"`
	MimeType    string    `db:"mime_type"`
	Data        []byte    `db:"data"`
	Score       int64     `db:"score"`
	SubmittedAt time.Time `db:"submitted_at"`
	SubmittedBy int64     `db:"submitted_by"`
	UpdatedAt   time.Time `db:"updated_at"`
	Flags       FlagBits  `db:"flags"`
	Metadata    Metadata  `db:"metadata"`
	Path        []byte    `db:"path"`
	FullPath    []byte
	author      *Account
}

func (i Item) Author() *Account {
	return i.author
}

func ItemFlags(f FlagBits) app.FlagBits {
	return VoteFlags(f)
}

func getAncestorKey(path []byte, cnt int) (app.Key, bool) {
	if path == nil {
		return app.Key{}, false
	}
	elem := bytes.Split(path, []byte("."))
	l := len(elem)
	if cnt > l || cnt < 0 {
		cnt = l
	}
	ls := elem[l-cnt]
	if len(ls) == 32 {
		var k app.Key
		i := copy(k[:], ls[0:32])
		return k, i == 32
	}
	return app.Key{}, false
}

func GetParentKey(i Item) (app.Key, bool) {
	return getAncestorKey(i.Path, 1)
}
func GetOPKey(i Item) (app.Key, bool) {
	return getAncestorKey(i.Path, -1)
}

func (i Item) Model() app.Item {
	a := i.Author().Model()
	am, _ := ItemMetadata(i.Metadata)
	res := app.Item{
		MimeType:    i.MimeType,
		SubmittedAt: i.SubmittedAt,
		SubmittedBy: &a,
		Metadata:    &am,
		Hash:        i.Key.Hash(),
		Flags:       ItemFlags(i.Flags),
		Path:        i.Path,
		Data:        string(i.Data),
		Title:       string(i.Title),
		Score:       i.Score,
		UpdatedAt:   i.UpdatedAt,
		IsTop:       len(i.Path) == 0,
	}
	if len(i.Path) > 0 {
		res.FullPath = append(i.Path, byte('.'))
		res.FullPath = append(res.FullPath, i.Key.Bytes()...)
	}
	if pKey, ok := GetParentKey(i); ok {
		res.Parent = &app.Item{
			Hash: pKey.Hash(),
		}
		if opKey, ok := GetOPKey(i); ok && pKey != opKey {
			res.OP = &app.Item{
				Hash: opKey.Hash(),
			}
		}
	}
	return res
}

type ItemCollection []Item

func saveItem(db *sqlx.DB, it app.Item) (app.Item, error) {
	i := Item{
		Score:    it.Score,
		MimeType: string(it.MimeType),
		Data:     []byte(it.Data),
		Title:    []byte(it.Title),
	}
	i.Metadata, _ = json.Marshal(it.Metadata)
	f := FlagBits{}
	if err := f.Scan(it.Flags); err == nil {
		i.Flags = f
	}

	if i.Key == [32]byte{} {
		i.Key = app.GenKey(i.Path, []byte(it.SubmittedBy.Handle), i.Data)
	}
	var params = make([]interface{}, 0)
	params = append(params, i.Key.Bytes())
	params = append(params, i.Title)
	params = append(params, i.Data)
	params = append(params, i.Metadata)
	params = append(params, i.MimeType)
	params = append(params, it.SubmittedBy.Hash)

	var ins string
	if it.Parent != nil && len(it.Parent.Hash) > 0 {
		ins = `insert into "content_items" ("key", "title", "data", "metadata", "mime_type", "submitted_by", "path") 
		values(
			$1, $2, $3, $4, $5, (select "id" from "accounts" where "key" ~* $6), (select (case when "path" is not null then concat("path", '.', "key") else "key" end) 
				as "parent_path" from "content_items" where key ~* $7)::ltree
		)`
		params = append(params, it.Parent.Hash)
	} else {
		ins = `insert into "content_items" ("key", "title", "data", "metadata", "mime_type", "submitted_by") 
		values($1, $2, $3, $4, $5, (select "id" from "accounts" where "key" ~* $6))`
	}

	res, err := db.Exec(ins, params...)
	if err != nil {
		return it, errors.Annotate(err, "db error")
	} else {
		if rows, _ := res.RowsAffected(); rows == 0 {
			return it, errors.Errorf("could not save item %q", i.Key.Hash())
		}
	}

	col, err := loadItems(db, app.LoadItemsFilter{Key: []string{i.Key.String()}, MaxItems: 1})
	if len(col) > 0 {
		return col[0], err
	} else {
		return app.Item{}, err
	}
}

type itemsView struct {
	ItemID          int64     `db:"item_id,"auto"`
	ItemKey         app.Key   `db:"item_key,size(32)"`
	Title           []byte    `db:"item_title"`
	MimeType        string    `db:"item_mime_type"`
	Data            []byte    `db:"item_data"`
	ItemScore       int64     `db:"item_score"`
	ItemSubmittedAt time.Time `db:"item_submitted_at"`
	ItemSubmittedBy int64     `db:"item_submitted_by"`
	ItemUpdatedAt   time.Time `db:"item_updated_at"`
	ItemFlags       FlagBits  `db:"item_flags"`
	ItemMetadata    Metadata  `db:"item_metadata"`
	Path            []byte    `db:"item_path"`
	AuthorID        int64     `db:"author_id,auto"`
	AuthorKey       app.Key   `db:"author_key,size(32)"`
	AuthorEmail     []byte    `db:"author_email"`
	AuthorHandle    string    `db:"author_handle"`
	AuthorScore     int64     `db:"author_score"`
	AuthorCreatedAt time.Time `db:"author_created_at"`
	AuthorUpdatedAt time.Time `db:"author_updated_at"`
	AuthorFlags     FlagBits  `db:"author_flags"`
	AuthorMetadata  Metadata  `db:"author_metadata"`
}

func (i itemsView) author() Account {
	return Account{
		Id:        i.AuthorID,
		Email:     i.AuthorEmail,
		Handle:    i.AuthorHandle,
		Key:       i.AuthorKey,
		CreatedAt: i.AuthorCreatedAt,
		UpdatedAt: i.AuthorUpdatedAt,
		Score:     i.AuthorScore,
		Flags:     i.AuthorFlags,
		Metadata:  i.AuthorMetadata,
	}
}

func (i itemsView) item() Item {
	author := i.author()
	return Item{
		ID:          i.ItemID,
		Key:         i.ItemKey,
		Title:       i.Title,
		Data:        i.Data,
		Path:        i.Path,
		SubmittedBy: i.ItemSubmittedBy,
		SubmittedAt: i.ItemSubmittedAt,
		UpdatedAt:   i.ItemUpdatedAt,
		MimeType:    i.MimeType,
		Score:       i.ItemScore,
		Flags:       i.ItemFlags,
		Metadata:    i.ItemMetadata,
		author:      &author,
	}
}

func loadItems(db *sqlx.DB, f app.LoadItemsFilter) (app.ItemCollection, error) {
	wheres, whereValues := f.GetWhereClauses("item", "author")
	var fullWhere string

	if len(wheres) == 0 {
		fullWhere = " true"
	} else if len(wheres) == 1 {
		fullWhere = fmt.Sprintf("%s", wheres[0])
	} else {
		fullWhere = fmt.Sprintf("(%s)", strings.Join(wheres, " AND "))
	}
	var offset string
	if f.Page > 0 {
		offset = fmt.Sprintf(" OFFSET %d", f.MaxItems*(f.Page-1))
	}
	// use hacker-news sort algorithm
	// (votes - 1) / pow((item_hour_age+2), gravity)
	sel := fmt.Sprintf(`select 
		"item"."id" as "item_id",
		"item"."key" as "item_key",
		"item"."mime_type" as "item_mime_type",
		"item"."data" as "item_data",
		"item"."title" as "item_title",
		"item"."score" as "item_score",
		"item"."submitted_at" as "item_submitted_at",
		"item"."submitted_by" as "item_submitted_by",
		"item"."flags" as "item_flags",
		"item"."metadata" as "item_metadata",
		"item"."path" as "item_path",
		"author"."id" as "author_id",
		"author"."key" as "author_key",
		"author"."handle" as "author_handle",
		"author"."email" as "author_email",
		"author"."score" as "author_score",
		"author"."created_at" as "author_created_at",
		"author"."metadata" as "author_metadata",
		"author"."flags" as "author_flags"
		from "content_items" as "item"
			left join "accounts" as "author" on "author"."id" = "item"."submitted_by" 
		where %s 
	order by 
		(("item"."score" - 1) / ((extract(epoch from age(current_timestamp, "item"."submitted_at")) / 3600.00) ^ %f))
	desc limit %d%s`, fullWhere, app.HNGravity, f.MaxItems, offset)

	agg := make([]itemsView, 0)
	if err := db.Select(&agg, sel, whereValues...); err != nil {
		return nil, err
	}
	items := make(app.ItemCollection, len(agg))
	for k, it := range agg {
		i := it.item().Model()
		items[k] = i
	}
	return items, nil
}
