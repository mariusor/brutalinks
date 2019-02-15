package db

import (
	"database/sql"
	"fmt"
	"github.com/go-pg/pg"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/internal/log"
	"math"
	"strings"
	"time"

	"github.com/juju/errors"
)

type VoteCollection map[app.Key]Vote

type Vote struct {
	Id          int64     `sql:"id"`
	SubmittedBy int64     `sql:"submitted_by"`
	SubmittedAt time.Time `sql:"submitted_at"`
	UpdatedAt   time.Time `sql:"updated_at"`
	ItemId      int64     `sql:"item_id"`
	Weight      int       `sql:"weight"`
	Flags       FlagBits  `sql:"flags"`
	item        *Item
	voter       *Account
}

func (v Vote) Voter() *Account {
	return v.voter
}
func (v Vote) Item() *Item {
	return v.item
}

func VoteFlags(f FlagBits) app.FlagBits {
	var ab uint8
	for _, b := range f {
		ab = ab | uint8(b)
	}
	return app.FlagBits(ab)
}

func (v Vote) Model() app.Vote {
	it := v.Item().Model()
	voter := v.Voter().Model()
	return app.Vote{
		Item:        &it,
		Weight:      v.Weight,
		UpdatedAt:   v.UpdatedAt,
		SubmittedAt: v.SubmittedAt,
		SubmittedBy: &voter,
		Flags:       VoteFlags(v.Flags),
	}
}

func (a Account) Votes() VoteCollection {
	return nil
}

func countVotes(db *pg.DB, f app.LoadVotesFilter) (uint, error) {
	wheres, whereValues := f.GetWhereClauses()
	var fullWhere string

	if len(wheres) == 0 {
		fullWhere = " true"
	} else if len(wheres) == 1 {
		fullWhere = fmt.Sprintf("%s", wheres[0])
	} else {
		fullWhere = fmt.Sprintf("(%s)", strings.Join(wheres, " AND "))
	}

	selC := fmt.Sprintf(`select count(*) from "votes" as "vote"
		inner join "accounts" as "voter" on "voter"."id" = "vote"."submitted_by"
		inner join "items" as "item" on "item"."id" = "vote"."item_id" 
		inner join "accounts" as "author" on "item"."submitted_by" = "author"."id"
		where %s`, fullWhere)
	var count uint
	if _, err := db.Query(&count, selC, whereValues...); err != nil {
		return 0, errors.Annotatef(err, "DB query error")
	}
	return count, nil
}

func loadVotes(db *pg.DB, f app.LoadVotesFilter) (app.VoteCollection, error) {
	wheres, whereValues := f.GetWhereClauses()
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
	selC := fmt.Sprintf(`select
		"vote"."id" as "vote_id",
		"vote"."weight" as "vote_weight",
		"vote"."submitted_at" as "vote_submitted_at",
		"vote"."flags" as "vote_flags",
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
		"voter"."id" as "voter_id",
		"voter"."key" as "voter_key",
		"voter"."handle" as "voter_handle",
		"voter"."email" as "voter_email",
		"voter"."score" as "voter_score",
		"voter"."created_at" as "voter_created_at",
		"voter"."metadata" as "voter_metadata",
		"voter"."flags" as "voter_flags",
		"author"."id" as "author_id",
		"author"."key" as "author_key",
		"author"."handle" as "author_handle",
		"author"."email" as "author_email",
		"author"."score" as "author_score",
		"author"."created_at" as "author_created_at",
		"author"."metadata" as "author_metadata",
		"author"."flags" as "author_flags"
	from "votes" as "vote"
		inner join "accounts" as "voter" on "voter"."id" = "vote"."submitted_by"
		inner join "items" as "item" on "item"."id" = "vote"."item_id" 
		inner join "accounts" as "author" on "item"."submitted_by" = "author"."id"
where %s order by "vote"."submitted_at" desc limit %d%s`, fullWhere, f.MaxItems, offset)
	agg := make([]votesView, 0)
	if _, err := db.Query(&agg, selC, whereValues...); err != nil {
		return nil, errors.Annotatef(err, "DB query error")
	}
	votes := make(app.VoteCollection, len(agg))
	for k, vv := range agg {
		v := vv.vote().Model()
		votes[k] = v
	}
	return votes, nil
}

type votesView struct {
	VoteID          int64               `sql:"vote_id"`
	VoteSubmittedBy int64               `sql:"vote_submitted_by"`
	VoteSubmittedAt time.Time           `sql:"vote_submitted_at"`
	VoteUpdatedAt   time.Time           `sql:"vote_updated_at"`
	Weight          int                 `sql:"vote_weight"`
	VoteFlags       FlagBits            `sql:"vote_flags"`
	ItemID          int64               `sql:"item_id,auto"`
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
	VoterID         int64               `sql:"voter_id,auto"`
	VoterKey        app.Key             `sql:"voter_key,size(32)"`
	VoterEmail      string              `sql:"voter_email"`
	VoterHandle     string              `sql:"voter_handle"`
	VoterScore      int64               `sql:"voter_score"`
	VoterCreatedAt  time.Time           `sql:"voter_created_at"`
	VoterUpdatedAt  time.Time           `sql:"voter_updated_at"`
	VoterFlags      FlagBits            `sql:"voter_flags"`
	VoterMetadata   app.AccountMetadata `sql:"voter_metadata"`
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

func (v votesView) author() Account {
	return Account{
		ID:        v.AuthorID,
		Email:     v.AuthorEmail,
		Handle:    v.AuthorHandle,
		Key:       v.AuthorKey,
		CreatedAt: v.AuthorCreatedAt,
		UpdatedAt: v.AuthorUpdatedAt,
		Score:     v.AuthorScore,
		Flags:     v.AuthorFlags,
		Metadata:  v.AuthorMetadata,
	}
}

func (v votesView) voter() Account {
	return Account{
		ID:        v.VoterID,
		Email:     v.VoterEmail,
		Handle:    v.VoterHandle,
		Key:       v.VoterKey,
		CreatedAt: v.VoterCreatedAt,
		UpdatedAt: v.VoterUpdatedAt,
		Score:     v.VoterScore,
		Flags:     v.VoterFlags,
		Metadata:  v.VoterMetadata,
	}
}

func (v votesView) vote() Vote {
	voter := v.voter()
	item := v.item()
	return Vote{
		Id:          v.VoteID,
		Weight:      v.Weight,
		ItemId:      v.ItemID,
		SubmittedBy: int64(math.Max(float64(v.VoteSubmittedBy), float64(v.VoterID))),
		SubmittedAt: v.VoteSubmittedAt,
		UpdatedAt:   v.VoteUpdatedAt,
		Flags:       v.VoteFlags,
		item:        &item,
		voter:       &voter,
	}
}

func (v votesView) item() Item {
	author := v.author()
	return Item{
		ID:          v.ItemID,
		Key:         v.ItemKey,
		Title:       v.Title,
		Data:        v.Data,
		Path:        v.Path,
		SubmittedBy: v.ItemSubmittedBy,
		SubmittedAt: v.ItemSubmittedAt,
		UpdatedAt:   v.ItemUpdatedAt,
		MimeType:    v.MimeType,
		Score:       v.ItemScore,
		Flags:       v.ItemFlags,
		Metadata:    v.ItemMetadata,
		author:      &author,
	}
}

func saveVote(db *pg.DB, vot app.Vote) (app.Vote, error) {
	var err error
	sel := `SELECT "votes"."id" as "vote_id", "accounts"."id" as "account_id", "votes"."weight" FROM "votes"
	INNER JOIN "accounts" ON "accounts"."id" = "votes"."submitted_by"
	WHERE "accounts"."key" ~* ?0 AND "votes"."item_id" = (SELECT "id" FROM "items" WHERE "key" ~* ?1);`

	old := struct {
		VoteID    int64 `sql:"vote_id"`
		AccountID int64 `sql:"account_id"`
		Weight    int64 `sql:"weight"`
	}{}
	db.QueryOne(&old, sel, vot.SubmittedBy.Hash, vot.Item.Hash)

	v := Vote{}
	var q string
	var updated bool
	if old.VoteID != 0 {
		if vot.Weight != 0 && old.Weight != 0 && math.Signbit(float64(old.Weight)) == math.Signbit(float64(vot.Weight)) {
			vot.Weight = 0
		}
		q = `update "votes" set "updated_at" = now(), "weight" = ?0, "flags" = ?1::bit(8) where "item_id" = (select "id" from "items" where "key" ~* ?2) and "submitted_by" = (select "id" from "accounts" where "key" ~* ?3);`
		updated = true
	} else {
		q = `insert into "votes" ("weight", "flags", "item_id", "submitted_by") values (?0, ?1::bit(8), (select "id" from "items" where "key" ~* ?2), (select "id" from "accounts" where "key" ~* ?3))`
	}
	v.Flags.Scan(vot.Flags)
	v.Weight = int(vot.Weight * app.ScoreMultiplier)

	res, err := db.Exec(q, v.Weight, app.FlagsNone, vot.Item.Hash, vot.SubmittedBy.Hash)
	if err != nil {
		return vot, err
	}
	if rows := res.RowsAffected(); rows == 0 || err != nil {
		return vot, errors.Errorf("scoring failed %s", err)
	}

	upd := `update "items" set score = score - ?0 + ?1 where "id" = (select "id" from "items" where "key" ~* ?2)`
	res, err = db.Exec(upd, v.Weight, v.Weight, vot.Item.Hash)
	if err != nil {
		return vot, err
	}
	if rows := res.RowsAffected(); rows == 0 {
		err = errors.Errorf("item corresponding to vote not found")
	}
	if rows := res.RowsAffected(); rows > 1 {
		err = errors.Errorf("item collision for vote")
	}
	if err == nil {
		Logger.WithContext(log.Ctx{
			"hash":      vot.Item.Hash,
			"updated":   updated,
			"oldWeight": old.Weight,
			"newWeight": vot.Weight,
			"voter":     vot.SubmittedBy.Hash,
		}).Info("vote updated successfully")
	} else {
		Logger.WithContext(log.Ctx{
			"hash":      vot.Item.Hash,
			"updated":   updated,
			"oldWeight": old.Weight,
			"newWeight": vot.Weight,
			"voter":     vot.SubmittedBy.Hash,
		}).Error(err.Error())
	}

	return vot, err
}
