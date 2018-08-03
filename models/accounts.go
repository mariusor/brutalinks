package models

import (
	"crypto/sha256"
	"fmt"
	"time"
	"database/sql"
	log "github.com/sirupsen/logrus"
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

func LoadAccount(db *sql.DB, handle string) (Account, error) {
	a := Account{}
	selAcct := `select "id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags" from "accounts" where "handle" = $1`
	rows, err := db.Query(selAcct, handle)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		err = rows.Scan(&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.UpdatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return a, err
		}
	}

	if err != nil {
		log.Print(err)
	}

	return a, nil
}

func LoadItemsSubmittedBy(db *sql.DB, handle string) ([]Content, error) {
	var err error
	p := Content{}
	items := make([]Content, 0)
	selC := `select 
			"content_items"."id", "content_items"."key", "content_items"."mime_type", "content_items"."data", 
			"content_items"."title", "content_items"."score", "content_items"."submitted_at", 
			"content_items"."submitted_by", "content_items"."flags", "content_items"."metadata", "content_items"."path",
			"accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score", 
			"accounts"."created_at", "accounts"."metadata", "accounts"."flags"
		from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
			where "accounts"."handle" = $1 order by "submitted_at" desc`
	{
		rows, err := db.Query(selC, handle)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			a := Account{}
			err := rows.Scan(
				&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
				&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
			if err != nil {
				return nil, err
			}
			p.SubmittedByAccount = a
			items = append(items, p)
		}
	}
	if err != nil {
		log.Error(err)
	}

	return items, nil
}
