package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"

	"github.com/mariusor/littr.go/app/models"
)

type Item struct {
	Id          int64     `db:"id,auto"`
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

func (i Item) Model() models.Item {
	a := i.Author().Model()
	fp := append(i.Path, byte('.'))
	fp = append(fp, i.Key.Bytes()...)
	return models.Item{
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
		Parent:      nil,
		OP:          nil,
		FullPath:    fp,
		IsTop:       len(i.Path) == 0,
	}
}

type ItemCollection []Item

func saveItem(db *sqlx.DB, it models.Item) (models.Item, error) {
	return it, errors.New("not implemented")
}

type itemsView struct {
	ItemId          int64     `db:"item_id,"auto"`
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
	AuthorId        int64     `db:"author_id,auto"`
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
		Id:        i.AuthorId,
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
		Id:          i.ItemId,
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
	wheres, whereValues := f.GetWhereClauses()
	var fullWhere string

	if len(wheres) == 0 {
		fullWhere = " true"
	} else if len(wheres) == 1 {
		fullWhere = fmt.Sprintf("%s", wheres[0])
	} else {
		fullWhere = fmt.Sprintf("(%s)", strings.Join(wheres, " AND "))
	}
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
	order by "item"."score" desc, "item"."submitted_at" desc limit %d`, fullWhere, f.MaxItems)

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
