package models

import (
	"crypto/sha256"
	"fmt"
	"time"
		log "github.com/sirupsen/logrus"
		"github.com/juju/errors"
	"encoding/json"
)

type AccountMetadata struct {
	Password []byte `json:"pw,omitempty"`
	Provider string `json:"provider,omitempty"`
	Salt     []byte `json:"salt,omitempty"`
}

type account struct {
	Id        int64     `orm:id,"auto"`
	Key       Key       `orm:key`
	Email     []byte    `orm:email`
	Handle    string    `orm:handle`
	Score     int64     `orm:score`
	CreatedAt time.Time `orm:created_at`
	UpdatedAt time.Time `orm:updated_at`
	Flags     int8      `orm:flags`
	Metadata  []byte    `orm:metadata`
	Votes     map[Key]vote
}

type Account struct {
	Email     string    `json:"-"`
	Hash      string    `json:"key"`
	Score     int64     `json:"score"`
	Handle    string    `json:"handle"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
	Flags     int8       `json:"-"`
	Metadata  AccountMetadata `json:"-"`
	Votes     map[string]Vote
}

func loadAccountFromModel (a account) Account {
	acct := Account{
		Hash:      a.Hash(),
		Flags:     a.Flags,
		UpdatedAt: a.UpdatedAt,
		Handle:    a.Handle,
		Score:     int64(float64(a.Score) / ScoreMultiplier),
		CreatedAt: a.CreatedAt,
		Email:     string(a.Email),
	}
	if a.Metadata != nil {
		err := json.Unmarshal(a.Metadata, &acct.Metadata)
		if err != nil {
			log.Error(errors.NewErrWithCause(err, "unable to unmarshal account metadata"))
		}
	}

	return acct
}

func loadAccount(handle string) (Account, error) {
	a, err := loadAccountByHandle(handle)
	if err != nil {
		return Account{}, errors.Errorf("user %q not found", handle)
	}
	return loadAccountFromModel(a), nil
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

func loadAccountByHandle(handle string) (account, error) {
	a := account{}
	selAcct := `select "id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags" from "accounts" where "handle" = $1`
	rows, err := Db.Query(selAcct, handle)
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
