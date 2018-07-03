package models

import "time"

type Vote struct {
	Id          int64     `orm:Id`
	SubmittedBy int64     `orm:submitted_by`
	SubmittedAt time.Time `orm:created_at`
	UpdatedAt   time.Time `orm:updated_at`
	ItemId      int64     `orm:item_id`
	Weight      int       `orm:Weight`
	Flags       int8      `orm:Flags`
}
func (v *Vote) IsYay () bool {
	return v != nil && v.Weight > 0
}
func (v *Vote) IsNay () bool {
	return v != nil && v.Weight < 0
}