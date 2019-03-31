package db

import (
	"bytes"
	"database/sql"
	"fmt"
	"github.com/go-pg/pg"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/internal/log"
	"strings"
	"time"

	"github.com/mariusor/littr.go/internal/errors"
)

type Item struct {
	ID          int64            `sql:"id,auto"`
	Key         app.Key          `sql:"key,size(32)"`
	Title       sql.NullString   `sql:"title"`
	MimeType    string           `sql:"mime_type"`
	Data        sql.NullString   `sql:"data"`
	Score       int64            `sql:"score"`
	SubmittedAt time.Time        `sql:"submitted_at"`
	SubmittedBy int64            `sql:"submitted_by"`
	UpdatedAt   time.Time        `sql:"updated_at"`
	Flags       FlagBits         `sql:"flags"`
	Metadata    app.ItemMetadata `sql:"metadata"`
	Path        Path             `sql:"path"`
	FullPath    Path
	author      *Account
}

func (i Item) Author() *Account {
	return i.author
}

func ItemFlags(f FlagBits) app.FlagBits {
	return VoteFlags(f)
}

func getAncestorKey(path Path, cnt int) (app.Key, bool) {
	if len(path) == 0 {
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
	am := i.Metadata
	res := app.Item{
		MimeType:    app.MimeType(i.MimeType),
		SubmittedAt: i.SubmittedAt,
		SubmittedBy: &a,
		Metadata:    &am,
		Hash:        i.Key.Hash(),
		Flags:       ItemFlags(i.Flags),
		Path:        i.Path,
		Data:        i.Data.String,
		Title:       i.Title.String,
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

var nilKey = [32]byte{}

type itemSaveError struct {
	item Item
	err  error
}

func (i itemSaveError) ItemCtx() log.Ctx {
	return log.Ctx{
		"ID":          i.item.ID,
		"Key":         i.item.Key,
		"Title":       i.item.Title,
		"MimeType":    i.item.MimeType,
		"Data":        i.item.Data,
		"Score":       i.item.Score,
		"SubmittedAt": i.item.SubmittedAt,
		"SubmittedBy": i.item.SubmittedBy,
		"UpdatedAt":   i.item.UpdatedAt,
		"Flags":       i.item.Flags,
		"Metadata":    i.item.Metadata,
		"Path":        i.item.Path,
	}
}

func (i itemSaveError) Error() string {
	return fmt.Sprintf("%s", i.err)
}

func keyComponents(it app.Item) [][]byte {
	keyFrom := [][]byte{
		it.Path,
		[]byte(it.Title),
		[]byte(it.Data),
		[]byte(it.SubmittedBy.Handle),
	}
	if it.MimeType != app.MimeTypeURL {
		keyFrom = append(keyFrom, []byte(it.UpdatedAt.String()))
	}
	return keyFrom
}

func saveItem(db *pg.DB, it app.Item) (app.Item, error) {
	i := Item{
		Score:    it.Score,
		MimeType: string(it.MimeType),
	}
	if len(it.Data) > 0 {
		i.Data.Scan(it.Data)
	}
	if len(it.Title) > 0 {
		i.Title.Scan(it.Title)
	}

	i.Metadata = *it.Metadata
	i.Flags.Scan(it.Flags)
	var params = make([]interface{}, 0)

	now := time.Now().UTC()
	if !it.SubmittedAt.IsZero() {
		i.SubmittedAt = it.SubmittedAt
	} else {
		i.SubmittedAt = now
	}
	if !it.UpdatedAt.IsZero() {
		i.UpdatedAt = it.UpdatedAt
	} else {
		i.UpdatedAt = now
	}

	var res pg.Result
	var err error
	var query string
	var hash app.Hash
	if len(it.Hash) == 0 {
		i.Key = app.GenKey(keyComponents(it)...)

		aKey := app.Key{}
		aKey.FromString(it.SubmittedBy.Hash.String())
		params = append(params, i.Key)
		params = append(params, i.Title)
		params = append(params, i.Data)
		params = append(params, i.Metadata)
		params = append(params, i.MimeType)
		params = append(params, i.SubmittedAt)
		params = append(params, i.UpdatedAt)
		params = append(params, i.Flags)
		params = append(params, aKey)

		if it.Parent != nil && len(it.Parent.Hash) > 0 {
			query = `INSERT INTO "items" ("key", "title", "data", "metadata", "mime_type", "submitted_at", "updated_at", "flags", "submitted_by", "path") 
		VALUES(
			?0, ?1, ?2, ?3, ?4, ?5, ?6, ?7::bit(8), (SELECT "id" FROM "accounts" WHERE "key" ~* ?8 OR "handle" = ?8), (SELECT (CASE WHEN "path" IS NOT NULL THEN concat("path", '.', "key") ELSE "key" END) 
				AS "parent_path" FROM "items" WHERE key ~* ?9)::ltree
		);`
			params = append(params, it.Parent.Hash)
		} else {
			query = `INSERT INTO "items" ("key", "title", "data", "metadata", "mime_type", "submitted_at", "updated_at", "flags", "submitted_by") 
		VALUES(?0, ?1, ?2, ?3, ?4, ?5, ?6, ?7::bit(8), (select "id" FROM "accounts" WHERE "key" ~* ?8 OR "handle" = ?8));`
		}
		hash = i.Key.Hash()
	} else {
		i.Key.FromString(it.Hash.String())

		params = append(params, i.Title)
		params = append(params, i.Data)
		params = append(params, i.Metadata)
		params = append(params, i.MimeType)
		params = append(params, i.Flags)
		params = append(params, now)
		params = append(params, i.Key)

		query = `UPDATE "items" SET "title" = ?0, "data" = ?1, "metadata" = ?2, "mime_type" = ?3,
			"flags" = ?4::bit(8), "updated_at" = ?5 WHERE "key" ~* ?6;`
		hash = i.Key.Hash()
	}
	res, err = db.Query(i, query, params...)
	if err != nil {
		return it, &itemSaveError{err: errors.Annotate(err, "item save error"), item: i}
	} else {
		if rows := res.RowsAffected(); rows == 0 {
			return it, &itemSaveError{err: errors.Errorf("could not save item %q", i.Key.Hash()), item: i}
		}
	}

	col, err := loadItems(db, app.Filters{LoadItemsFilter: app.LoadItemsFilter{Key: app.Hashes{hash}}, MaxItems: 1})
	if len(col) > 0 {
		return col[0], nil
	} else {
		return app.Item{}, &itemSaveError{err: errors.Annotate(err, "item save error"), item: i}
	}
}

type itemsView struct {
	ItemID          int64               `sql:"item_id,"auto"`
	ItemKey         app.Key             `sql:"item_key,size(32)"`
	Title           sql.NullString      `sql:"item_title"`
	MimeType        string              `sql:"item_mime_type"`
	Data            sql.NullString      `sql:"item_data"`
	ItemScore       int64               `sql:"item_score"`
	ItemSubmittedAt time.Time           `sql:"item_submitted_at"`
	ItemSubmittedBy int64               `sql:"item_submitted_by"`
	ItemUpdatedAt   time.Time           `sql:"item_updated_at"`
	ItemFlags       FlagBits            `sql:"item_flags"`
	ItemMetadata    app.ItemMetadata    `sql:"item_metadata"`
	Path            Path                `sql:"item_path"`
	AuthorID        int64               `sql:"author_id,auto"`
	AuthorKey       app.Key             `sql:"author_key,size(32)"`
	AuthorEmail     string              `sql:"author_email"`
	AuthorHandle    string              `sql:"author_handle"`
	AuthorScore     int64               `sql:"author_score"`
	AuthorCreatedAt time.Time           `sql:"author_created_at"`
	AuthorUpdatedAt time.Time           `sql:"author_updated_at"`
	AuthorFlags     FlagBits            `sql:"author_flags"`
	AuthorMetadata  app.AccountMetadata `sql:"author_metadata"`
}

func (i itemsView) author() Account {
	return Account{
		ID:        i.AuthorID,
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

func countItems(db *pg.DB, f app.Filters) (uint, error) {
	wheres, whereValues := f.WithAuthorAlias("author").WithContentAlias("item").GetWhereClauses()
	var fullWhere string

	if len(wheres) == 0 {
		fullWhere = " true"
	} else if len(wheres) == 1 {
		fullWhere = fmt.Sprintf("%s", wheres[0])
	} else {
		fullWhere = fmt.Sprintf("(%s)", strings.Join(wheres, " AND "))
	}

	selC := fmt.Sprintf(`select count(*) from "items" as "item"
			left join "accounts" as "author" on "author"."id" = "item"."submitted_by" 
		where %s`, fullWhere)
	var count uint
	if _, err := db.Query(&count, selC, whereValues...); err != nil {
		return 0, errors.Annotatef(err, "DB query error")
	}
	return count, nil
}

func loadItems(db *pg.DB, f app.Filters) (app.ItemCollection, error) {
	wheres, whereValues := f.WithAuthorAlias("author").WithContentAlias("item").GetWhereClauses()
	var fullWhere string

	if len(wheres) == 0 {
		fullWhere = " true"
	} else if len(wheres) == 1 {
		fullWhere = fmt.Sprintf("%s", wheres[0])
	} else {
		fullWhere = fmt.Sprintf("(%s)", strings.Join(wheres, " AND "))
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
		from "items" as "item"
			left join "accounts" as "author" on "author"."id" = "item"."submitted_by" 
		where %s 
	order by 
		(("item"."score" - 1) / ((extract(epoch from age(current_timestamp, "item"."submitted_at")) / 3600.00) ^ %f))
	desc%s`, fullWhere, app.HNGravity, f.GetLimit())

	agg := make([]itemsView, 0)
	items := make(app.ItemCollection, 0)
	if _, err := db.Query(&agg, sel, whereValues...); err != nil {
		return items, errors.Annotatef(err, "DB query error")
	}
	for _, it := range agg {
		i := it.item().Model()
		items = append(items, i)
	}
	return items, nil
}
