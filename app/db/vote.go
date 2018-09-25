package db

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app/models"
)

type VoteCollection map[Key]Vote

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

func VoteFlags(f FlagBits) models.FlagBits {
	var ab uint8 = 0
	for _, b := range f {
		ab = ab & uint8(b)
	}
	return models.FlagBits(ab)
}

func (v Vote) Model() models.Vote {
	it := v.Item().Model()
	voter := v.Voter().Model()
	return models.Vote{
		Item:        &it,
		SubmittedBy: &voter,
		Flags:       VoteFlags(v.Flags),
	}
}

func (a Account) Votes() VoteCollection {
	return nil
}

func loadVotes(db *sqlx.DB, filter models.LoadVotesFilter) (models.VoteCollection, error) {
	wheres, whereValues := filter.GetWhereClauses()

	fullWhere := fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(wheres, " AND ")))
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
where %s order by "vote"."submitted_at" desc limit %d`, fullWhere, filter.MaxItems)
	agg := make([]votesView, 0)
	udb := db.Unsafe()
	if err := udb.Select(&agg, selC, whereValues...); err != nil {
		return nil, err
	}
	votes := make(models.VoteCollection, len(agg))
	for _, vv := range agg {
		i := vv.item().Model()
		v := vv.vote().Model()
		votes[i.Hash] = v
	}
	return votes, nil
}

type votesView struct {
	VoteId          int64     `db:"vote_id"`
	VoteSubmittedBy int64     `db:"vote_submitted_by"`
	VoteSubmittedAt time.Time `db:"vote_submitted_at"`
	VoteUpdatedAt   time.Time `db:"vote_updated_at"`
	Weight          int       `db:"vote_weight"`
	VoteFlags       FlagBits  `db:"vote_flags"`
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
	VoterId         int64     `db:"voter_id,auto"`
	VoterKey        Key       `db:"voter_key,size(64)"`
	VoterEmail      []byte    `db:"voter_email"`
	VoterHandle     string    `db:"voter_handle"`
	VoterScore      int64     `db:"voter_score"`
	VoterCreatedAt  time.Time `db:"voter_created_at"`
	VoterUpdatedAt  time.Time `db:"voter_updated_at"`
	VoterFlags      FlagBits  `db:"voter_flags"`
	VoterMetadata   Metadata  `db:"voter_metadata"`
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

func (v votesView) author() Account {
	return Account{
		Id:        v.AuthorId,
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
		Id:        v.VoterId,
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
		Id:          v.VoteId,
		Weight:      v.Weight,
		ItemId:      v.ItemId,
		SubmittedBy: int64(math.Max(float64(v.VoteSubmittedBy), float64(v.VoterId))),
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
		Id:          v.ItemId,
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

func saveVote(db *sqlx.DB, v models.Vote) (models.Vote, error) {
	return v, errors.New("not implemented")
}
