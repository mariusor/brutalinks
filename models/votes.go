package models

import (
	"time"
	"database/sql"
	log "github.com/sirupsen/logrus"
)

const (
	ScoreMultiplier = 10000
	ScoreMaxK       = 10000.0
	ScoreMaxM       = 10000000.0
	ScoreMaxB       = 10000000000.0
)

type Vote struct {
	Id          int64     `orm:Id`
	SubmittedBy int64     `orm:submitted_by`
	SubmittedAt time.Time `orm:created_at`
	UpdatedAt   time.Time `orm:updated_at`
	ItemId      int64     `orm:item_id`
	Weight      int       `orm:Weight`
	Flags       int8      `orm:Flags`
}

func LoadItemsAndVotesSubmittedBy(db *sql.DB, handle string) (*[]Content, *[]Vote, error) {
	var err error
	items := make([]Content, 0)
	votes := make([]Vote, 0)
	selC := `select 
		"votes"."id", 
		"votes"."weight", 
		"votes"."submitted_at", 
		"votes"."flags",
		"content_items"."id", 
		"content_items"."key", 
		"content_items"."mime_type", 
		"content_items"."data", 
		"content_items"."title", 
		"content_items"."score",
		"content_items"."submitted_at", 
		"content_items"."submitted_by",
		"content_items"."flags", 
		"content_items"."metadata", 
		"accounts"."id"
from "content_items"
  inner join "votes" on "content_items"."id" = "votes"."item_id"
  left join "accounts" on "accounts"."id" = "content_items"."submitted_by"
where "accounts"."handle" = $1 order by "votes"."submitted_at" desc`
	{
		rows, err := db.Query(selC, handle)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			v := Vote{}
			p := Content{}
			a := Account{}
			err = rows.Scan(
				&v.Id,
				&v.Weight,
				&v.SubmittedAt,
				&v.Flags,
				&p.Id,
				&p.Key,
				&p.MimeType,
				&p.Data,
				&p.Title,
				&p.Score,
				&p.SubmittedAt,
				&p.SubmittedBy,
				&p.Flags,
				&p.Metadata,
				&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
			if err != nil {
				return nil, nil, err
			}
			p.SubmittedByAccount = a
			items = append(items, p)
			votes = append(votes, v)
		}
	}
	if err != nil {
		log.Error(err)
	}
	return &items, &votes, nil
}
