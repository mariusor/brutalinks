package db

import (
	"fmt"
	"github.com/mariusor/littr.go/app"
	"math"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
)

type VoteCollection map[app.Key]Vote

type Vote struct {
	Id          int64     `db:"id"`
	SubmittedBy int64     `db:"submitted_by"`
	SubmittedAt time.Time `db:"submitted_at"`
	UpdatedAt   time.Time `db:"updated_at"`
	ItemId      int64     `db:"item_id"`
	Weight      int       `db:"weight"`
	Flags       FlagBits  `db:"flags"`
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
		ab = ab & uint8(b)
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

func loadVotes(db *sqlx.DB, f app.LoadVotesFilter) (app.VoteCollection, error) {
	wheres, whereValues := f.GetWhereClauses()

	fullWhere := fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(wheres, " AND ")))

	var offset string
	if f.Page > 0 {
		offset = fmt.Sprintf(" OFFSEt %d", f.MaxItems*(f.Page-1))
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
		inner join "content_items" as "item" on "item"."id" = "vote"."item_id" 
		inner join "accounts" as "author" on "item"."submitted_by" = "author"."id"
where %s order by "vote"."submitted_at" desc limit %d%s`, fullWhere, f.MaxItems, offset)
	agg := make([]votesView, 0)
	udb := db.Unsafe()
	if err := udb.Select(&agg, selC, whereValues...); err != nil {
		return nil, err
	}
	votes := make(app.VoteCollection, len(agg))
	for _, vv := range agg {
		i := vv.item().Model()
		v := vv.vote().Model()
		votes[i.Hash] = v
	}
	return votes, nil
}

type votesView struct {
	VoteID          int64      `db:"vote_id"`
	VoteSubmittedBy int64      `db:"vote_submitted_by"`
	VoteSubmittedAt time.Time  `db:"vote_submitted_at"`
	VoteUpdatedAt   time.Time  `db:"vote_updated_at"`
	Weight          int        `db:"vote_weight"`
	VoteFlags       FlagBits   `db:"vote_flags"`
	ItemID          int64      `db:"item_id,"auto"`
	ItemKey         app.Key `db:"item_key,size(32)"`
	Title           []byte     `db:"item_title"`
	MimeType        string     `db:"item_mime_type"`
	Data            []byte     `db:"item_data"`
	ItemScore       int64      `db:"item_score"`
	ItemSubmittedAt time.Time  `db:"item_submitted_at"`
	ItemSubmittedBy int64      `db:"item_submitted_by"`
	ItemUpdatedAt   time.Time  `db:"item_updated_at"`
	ItemFlags       FlagBits   `db:"item_flags"`
	ItemMetadata    Metadata   `db:"item_metadata"`
	Path            []byte     `db:"item_path"`
	VoterID         int64      `db:"voter_id,auto"`
	VoterKey        app.Key `db:"voter_key,size(32)"`
	VoterEmail      []byte     `db:"voter_email"`
	VoterHandle     string     `db:"voter_handle"`
	VoterScore      int64      `db:"voter_score"`
	VoterCreatedAt  time.Time  `db:"voter_created_at"`
	VoterUpdatedAt  time.Time  `db:"voter_updated_at"`
	VoterFlags      FlagBits   `db:"voter_flags"`
	VoterMetadata   Metadata   `db:"voter_metadata"`
	AuthorID        int64      `db:"author_id,auto"`
	AuthorKey       app.Key `db:"author_key,size(32)"`
	AuthorEmail     []byte     `db:"author_email"`
	AuthorHandle    string     `db:"author_handle"`
	AuthorScore     int64      `db:"author_score"`
	AuthorCreatedAt time.Time  `db:"author_created_at"`
	AuthorUpdatedAt time.Time  `db:"author_updated_at"`
	AuthorFlags     FlagBits   `db:"author_flags"`
	AuthorMetadata  Metadata   `db:"author_metadata"`
}

func (v votesView) author() Account {
	return Account{
		Id:        v.AuthorID,
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
		Id:        v.VoterID,
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

func saveVote(db *sqlx.DB, vot app.Vote) (app.Vote, error) {
	var sel string
	sel = `select "votes"."id", "accounts"."id", "votes"."weight", "votes"."submitted_at" from "votes" inner join "accounts" on "accounts"."id" = "votes"."submitted_by" 
			where "accounts"."key" ~* $1 and "votes"."item_id" = (select "id" from "content_items" where "key" ~* $2);`
	var userID int64
	var vID int64
	var oldWeight int64
	var submittedAt time.Time

	var err error
	rows, err := db.Query(sel, vot.SubmittedBy.Hash, vot.Item.Hash)
	if err != nil {
		return vot, err
	}
	for rows.Next() {
		err = rows.Scan(&vID, &userID, &oldWeight, &submittedAt)
		if err != nil {
			return vot, err
		}
	}
	vot.SubmittedAt = submittedAt

	v := Vote{}
	var q string
	var updated bool
	if vID != 0 {
		if vot.Weight != 0 && oldWeight != 0 && math.Signbit(float64(oldWeight)) == math.Signbit(float64(vot.Weight)) {
			vot.Weight = 0
		}
		q = `update "votes" set "updated_at" = now(), "weight" = $1, "flags" = $2::bit(8) where "item_id" = (select "id" from "content_items" where "key" ~* $3) and "submitted_by" = (select "id" from "accounts" where "key" ~* $4);`
		updated = true
	} else {
		q = `insert into "votes" ("weight", "flags", "item_id", "submitted_by") values ($1, $2::bit(8), (select "id" from "content_items" where "key" ~* $3), (select "id" from "accounts" where "key" ~* $4))`
	}
	v.Flags.Scan(vot.Flags)
	v.Weight = int(vot.Weight * app.ScoreMultiplier)

	res, err := db.Exec(q, v.Weight, app.FlagsNone, vot.Item.Hash, vot.SubmittedBy.Hash)
	if err != nil {
		return vot, err
	}
	if rows, err := res.RowsAffected(); rows == 0 || err != nil {
		return vot, errors.Errorf("scoring failed %s", err)
	}

	upd := `update "content_items" set score = score - $1 + $2 where "id" = (select "id" from "content_items" where "key" ~* $3)`
	res, err = db.Exec(upd, v.Weight, v.Weight, vot.Item.Hash)
	if err != nil {
		return vot, err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		err = errors.Errorf("item corresponding to vote not found")
	}
	if rows, _ := res.RowsAffected(); rows > 1 {
		err = errors.Errorf("item collision for vote")
	}
	if err == nil {
		log.WithFields(log.Fields{
			"hash":      vot.Item.Hash,
			"updated":   updated,
			"oldWeight": oldWeight,
			"newWeight": vot.Weight,
			"voter":     vot.SubmittedBy.Hash,
		}).Infof("vote updated successfully")
	} else {
		log.WithFields(log.Fields{
			"hash":      vot.Item.Hash,
			"updated":   updated,
			"oldWeight": oldWeight,
			"newWeight": vot.Weight,
			"voter":     vot.SubmittedBy.Hash,
		}).Error(err)
	}

	return vot, err
}
