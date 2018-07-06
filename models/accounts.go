package models

import (
	"fmt"
	"time"
)

type Account struct {
	Id        int64     `orm:Id,"auto"`
	Key       string    `orm:key`
	Email     string    `orm:email`
	Handle    string    `orm:handle`
	Score     int64     `orm:score`
	CreatedAt time.Time `orm:created_at`
	UpdatedAt time.Time `orm:updated_at`
	Flags     int8      `orm:Flags`
	Metadata  []byte    `orm:metadata`
	Votes     map[int64]Vote
}

type CanDelete interface {
	Deleted() bool
	Delete()
	UnDelete()
}

func (a *Account) VotedOn(i Content) *Vote {
	for _, v := range a.Votes {
		if v.ItemId == i.Id {
			return &v
		}
	}
	return nil
}

const anonymous = "anonymous"

func AnonymousAccount() Account {
	return Account{Id: 0, Handle: anonymous, Votes: make(map[int64]Vote)}
}

func (a Account) Hash() string {
	return a.Hash8()
}
func (a Account) Hash8() string {
	return string(a.Key[0:8])
}
func (a Account) Hash16() string {
	return string(a.Key[0:16])
}
func (a Account) Hash32() string {
	return string(a.Key[0:32])
}
func (a Account) Hash64() string {
	return string(a.Key)
}
func (a Account) PermaLink() string {
	return fmt.Sprintf("/~%s", a.Handle)
}
