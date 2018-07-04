package models

import (
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

func (u *Account) VotedOn(i Content) *Vote {
	for _, v := range u.Votes {
		if v.ItemId == i.Id {
			return &v
		}
	}
	return nil
}

const anonymous = "anonymous"

func AnonymousAccount() Account {
	return Account{Id: -1, Handle: anonymous, Votes: make(map[int64]Vote)}
}
