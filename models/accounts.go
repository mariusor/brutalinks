package models

import (
	"crypto/sha256"
	"fmt"
	"time"
	"database/sql"
	log "github.com/sirupsen/logrus"
		"github.com/juju/errors"
)

type AccountMetadata struct {
	Password []byte `json:"pw,omitemtpty"`
	Provider string `json:"provider,omitempty"`
	Salt     []byte `json:"salt,omitempty"`
}

type account struct {
	Id        int64     `orm:Id,"auto"`
	Key       Key       `orm:key`
	Email     []byte    `orm:email`
	Handle    string    `orm:handle`
	Score     int64     `orm:score`
	CreatedAt time.Time `orm:created_at`
	UpdatedAt time.Time `orm:updated_at`
	Flags     int8      `orm:Flags`
	Metadata  []byte    `orm:metadata`
	Votes     map[Key]Vote
}

type Account struct {
	Hash      string    `json:"key"`
	Email     []byte    `json:"email"`
	Handle    string    `json:"handle"`
	Score     int64     `json:"score"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Flags     int8
	Metadata  []byte
	Votes     map[string]Vote
}

func loadFromModel (a account) Account {
	return Account{
		Hash:      a.Hash(),
		Flags:     a.Flags,
		UpdatedAt: a.UpdatedAt,
		Handle:    a.Handle,
		Metadata:  a.Metadata, // this needs processing
		Score:     int64(float64(a.Score) / ScoreMultiplier),
		CreatedAt: a.CreatedAt,
		Email:     a.Email,
	}
}

func LoadAccount(handle string) (Account, error) {
	a, err := LoadAccountByHandle(Db, handle)
	if err != nil {
		return Account{}, errors.Errorf("user %q not found", handle)
	}
	return loadFromModel(a), nil
}

type Deletable interface {
	Deleted() bool
	Delete()
	UnDelete()
}

func (a *Account) VotedOn(i Item) *Vote {
	for key, v := range a.Votes {
		if key == i.Hash {
			return &v
		}
	}
	return nil
}
func (a *account) VotedOn(i item) *Vote {
	for _, v := range a.Votes {
		if v.ItemId == i.Id {
			return &v
		}
	}
	return nil
}

func (a account) Hash() string {
	return a.Hash8()
}
func (a account) Hash8() string {
	return string(a.Key[0:8])
}
func (a account) Hash16() string {
	return string(a.Key[0:16])
}
func (a account) Hash32() string {
	return string(a.Key[0:32])
}
func (a account) Hash64() string {
	return a.Key.String()
}

func (a Account) GetLink() string {
	return fmt.Sprintf("/~%s", a.Handle)
}

func GenKey(handle string) Key {
	data := []byte(handle)
	//now := a.UpdatedAt
	//if now.IsZero() {
	//	now = time.Now()
	//}
	k := Key{}
	k.FromString(fmt.Sprintf("%x", sha256.Sum256(data)))
	return k
}

func (a *Account) IsLogged() bool {
	return a != nil && (!a.CreatedAt.IsZero())
}

func LoadAccountByHandle(db *sql.DB, handle string) (account, error) {
	a := account{}
	selAcct := `select "id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags" from "accounts" where "handle" = $1`
	rows, err := db.Query(selAcct, handle)
	if err != nil {
		return a, err
	}
	var aKey []byte
	for rows.Next() {
		err = rows.Scan(&a.Id, &aKey, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.UpdatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return a, err
		}
		a.Key.FromBytes(aKey)
	}

	if err != nil {
		log.Print(err)
	}

	return a, nil
}

func LoadItemsSubmittedBy(handle string) (ItemCollection, error) {
	var err error
	p := item{}
	items := make(ItemCollection, 0)
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
		rows, err := Db.Query(selC, handle)
		if err != nil {
			return nil, err
		}
		var aKey, pKey []byte
		for rows.Next() {
			a := account{}
			err := rows.Scan(
				&p.Id, &pKey, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
				&a.Id, &aKey, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
			if err != nil {
				return nil, err
			}

			p.Key.FromBytes(pKey)
			a.Key.FromBytes(aKey)
			acct := loadFromModel(a)
			p.SubmittedByAccount = &acct
			items = append(items, loadItemFromContent(p))
		}
	}
	if err != nil {
		log.Error(err)
	}

	return items, nil
}
