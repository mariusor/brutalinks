package db

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"

	"github.com/mariusor/littr.go/app/models"
)

type Item struct {
	ID          int64     `db:"id,auto"`
	Key         Key       `db:"key,size(64)"`
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

func ItemFlags(f FlagBits) models.FlagBits {
	return VoteFlags(f)
}

func getAncestorKey(path []byte, cnt int) (Key, bool) {
	if path == nil {
		return Key{}, false
	}
	elem := bytes.Split(path, []byte("."))
	l := len(elem)
	if cnt > l || cnt < 0 {
		cnt = l
	}
	ls := elem[l-cnt]
	if len(ls) == 64 {
		var k Key
		i := copy(k[:], ls[0:64])
		return k, i == 64
	}
	return Key{}, false
}

func GetParentKey(i Item) (Key, bool) {
	return getAncestorKey(i.Path, 1)
}
func GetOPKey(i Item) (Key, bool) {
	return getAncestorKey(i.Path, -1)
}

func (i Item) Model() models.Item {
	a := i.Author().Model()
	res := models.Item{
		MimeType:    i.MimeType,
		SubmittedAt: i.SubmittedAt,
		SubmittedBy: &a,
		Metadata:    ItemMetadata(i.Metadata),
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
		res.Parent = &models.Item{
			Hash: pKey.Hash(),
		}
		if opKey, ok := GetOPKey(i); ok && pKey != opKey {
			res.OP = &models.Item{
				Hash: opKey.Hash(),
			}
		}
	}
	return res
}

type ItemCollection []Item

func genKey(i Item) Key {
	data := i.Data
	now := i.UpdatedAt
	if now.IsZero() {
		now = time.Now()
	}
	data = append(data, []byte(fmt.Sprintf("%d", now.UnixNano()))...)
	data = append(data, []byte(i.Path)...)
	data = append(data, []byte(fmt.Sprintf("%d", i.SubmittedBy))...)

	i.Key.FromString(fmt.Sprintf("%x", sha256.Sum256(data)))
	return i.Key
}

func saveItem(db *sqlx.DB, it models.Item) (models.Item, error) {
	i := Item{
		Score:    it.Score,
		MimeType: it.MimeType,
		Data:     []byte(it.Data),
		Title:    []byte(it.Title),
	}
	if it.Metadata != nil {
		i.Metadata, _ = json.Marshal(it.Metadata)
	}
	f := FlagBits{}
	if err := f.Scan(it.Flags); err == nil {
		i.Flags = f
	}

	if i.Key == [64]byte{} {
		i.Key = genKey(i)
	}
	var params = make([]interface{}, 0)
	params = append(params, i.Key.Bytes())
	params = append(params, i.Title)
	params = append(params, i.Data)
	params = append(params, i.MimeType)
	params = append(params, it.SubmittedBy.Hash)

	var ins string
	if it.Parent != nil && len(it.Parent.Hash) > 0 {
		ins = `insert into "content_items" ("key", "title", "data", "mime_type", "submitted_by", "path") 
		values(
			$1, $2, $3, $4, (select "id" from "accounts" where "key" ~* $5), (select (case when "path" is not null then concat("path", '.', "key") else "key" end) 
				as "parent_path" from "content_items" where key ~* $6)::ltree
		)`
		params = append(params, it.Parent.Hash)
	} else {
		ins = `insert into "content_items" ("key", "title", "data", "mime_type", "submitted_by") 
		values($1, $2, $3, $4, (select "id" from "accounts" where "key" ~* $5))`
	}

	res, err := db.Exec(ins, params...)
	if err != nil {
		return it, err
	} else {
		if rows, _ := res.RowsAffected(); rows == 0 {
			return it, errors.Errorf("could not save item %q", i.Key.Hash())
		}
	}

	col, err := loadItems(db, models.LoadItemsFilter{Key: []string{i.Key.String()}, MaxItems: 1})
	if len(col) > 0 {
		return col[0], err
	} else {
		return models.Item{}, err
	}
}

type itemsView struct {
	ItemID          int64     `db:"item_id,"auto"`
	ItemKey         Key       `db:"item_key,size(64)"`
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
	AuthorKey       Key       `db:"author_key,size(64)"`
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

func loadItems(db *sqlx.DB, f models.LoadItemsFilter) (models.ItemCollection, error) {
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
		offset = fmt.Sprintf(" OFFSEt %d", f.MaxItems*(f.Page-1))
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
	desc limit %d%s`, fullWhere, models.HNGravity, f.MaxItems, offset)

	agg := make([]itemsView, 0)
	if err := db.Select(&agg, sel, whereValues...); err != nil {
		return nil, err
	}
	items := make(models.ItemCollection, len(agg))
	for k, it := range agg {
		i := it.item().Model()
		items[k] = i
	}
	return items, nil
}
