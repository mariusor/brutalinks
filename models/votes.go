package models

import "time"

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
