package models

import (
	"crypto/sha256"
	"fmt"
	"time"
)

type AccountMetadata struct {
	Password []byte `json:"pw,omitemtpty"`
	Provider string `json:"provider,omitempty"`
	Salt     []byte `json:"salt,omitempty"`
}

type Account struct {
	Id        int64     `orm:Id,"auto"`
	Key       []byte    `orm:key`
	Email     []byte    `orm:email`
	Handle    string    `orm:handle`
	Score     int64     `orm:score`
	CreatedAt time.Time `orm:created_at`
	UpdatedAt time.Time `orm:updated_at`
	Flags     int8      `orm:Flags`
	Metadata  []byte    `orm:metadata`
	Votes     map[int64]Vote
}

type Deletable interface {
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

func (a Account) GetLink() string {
	return fmt.Sprintf("/~%s", a.Handle)
}

func (a Account) GetKey() []byte {
	data := []byte(a.Handle)
	now := a.UpdatedAt
	if now.IsZero() {
		now = time.Now()
	}

	a.Key = []byte(fmt.Sprintf("%x", sha256.Sum256(data)))
	return a.Key
}
