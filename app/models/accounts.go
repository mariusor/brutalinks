package models

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
)

type SSHKey struct {
	Id      string `json:"id"`
	Private []byte `json:"prv,omitempty"`
	Public  []byte `json:"pub,omitempty"`
}

type AccountMetadata struct {
	Password []byte  `json:"pw,omitempty"`
	Provider string  `json:"provider,omitempty"`
	Salt     []byte  `json:"salt,omitempty"`
	Key      *SSHKey `json:"key,omitempty"`
}

type account struct {
	Id        int64     `orm:id,"auto"`
	Key       Key       `orm:key`
	Email     []byte    `orm:email`
	Handle    string    `orm:handle`
	Score     int64     `orm:score`
	CreatedAt time.Time `orm:created_at`
	UpdatedAt time.Time `orm:updated_at`
	Flags     FlagBits  `orm:flags`
	Metadata  []byte    `orm:metadata`
	Votes     map[Key]vote
}

type AccountCollection []Account

type Account struct {
	Email     string           `json:"email,omitempty"`
	Hash      string           `json:"hash,omitempty"`
	Score     int64            `json:"score,omitempty"`
	Handle    string           `json:"handle"`
	CreatedAt time.Time        `json:"-"`
	UpdatedAt time.Time        `json:"-"`
	Flags     FlagBits         `json:"flags,omitempty"`
	Metadata  *AccountMetadata `json:"metadata,omitempty"`
	Votes     map[string]Vote  `json:"votes,omitempty"`
}

func (a Account) HasMetadata() bool {
	return a.Metadata != nil
}

func (a Account) IsValid() bool {
	return len(a.Handle) > 0
}

func loadAccountFromModel(a account) Account {
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
			log.WithFields(log.Fields{}).Error(errors.NewErrWithCause(err, "unable to unmarshal account metadata"))
		}
	}

	return acct
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

func loadAccount(db *sql.DB, filter LoadAccountFilter) (Account, error) {
	var wheres []string
	whereValues := make([]interface{}, 0)
	counter := 1
	if len(filter.Key) > 0 {
		whereColumns := make([]string, 0)
		//for _, hash := range filter.Key {
		whereColumns = append(whereColumns, fmt.Sprintf(`"accounts"."key" ~* $%d`, counter))
		whereValues = append(whereValues, interface{}(filter.Key))
		counter += 1
		//}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	if len(filter.Handle) > 0 {
		whereColumns := make([]string, 0)
		//for _, hash := range filter.Handle {
		whereColumns = append(whereColumns, fmt.Sprintf(`"accounts"."handle" ~* $%d`, counter))
		whereValues = append(whereValues, interface{}(filter.Handle))
		counter += 1
		//}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	a := account{}
	var acct Account
	var fullWhere string
	if len(wheres) > 0 {
		fullWhere = fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(wheres, " AND ")))
	} else {
		fullWhere = " true"
	}
	sel := fmt.Sprintf(`select 
		"id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags"
	from "accounts" where %s`, fullWhere)
	rows, err := db.Query(sel, whereValues...)
	if err != nil {
		return acct, err
	}

	var aKey []byte
	for rows.Next() {
		err = rows.Scan(&a.Id, &aKey, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.UpdatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return acct, err
		}
		a.Key.FromBytes(aKey)
	}

	acct = loadAccountFromModel(a)
	if len(a.Handle) == 0 {
		return acct, errors.Errorf("not found")
	}
	return acct, nil
}

func loadAccounts(db *sql.DB, filter LoadAccountsFilter) (AccountCollection, error) {
	var wheres []string
	whereValues := make([]interface{}, 0)
	counter := 1

	accounts := make(AccountCollection, 0)

	if len(filter.Key) > 0 {
		whereColumns := make([]string, 0)
		for _, hash := range filter.Key {
			whereColumns = append(whereColumns, fmt.Sprintf(`"accounts"."key" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(hash))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	if len(filter.Handle) > 0 {
		whereColumns := make([]string, 0)
		for _, handle := range filter.Handle {
			whereColumns = append(whereColumns, fmt.Sprintf(`"accounts"."handle" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(handle))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	a := account{}
	var acct Account
	var fullWhere string
	if len(wheres) > 0 {
		fullWhere = fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(wheres, " AND ")))
	} else {
		fullWhere = " true"
	}
	sel := fmt.Sprintf(`select 
		"id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags"
	from "accounts" where %s`, fullWhere)
	rows, err := db.Query(sel, whereValues...)
	if err != nil {
		return accounts, err
	}

	var aKey []byte
	for rows.Next() {
		err = rows.Scan(&a.Id, &aKey, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.UpdatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return accounts, err
		}
		if len(aKey) == 0 {
			continue
		}
		a.Key.FromBytes(aKey)
		acct = loadAccountFromModel(a)
		accounts = append(accounts, acct)
	}

	return accounts, nil
}

func saveAccount(db *sql.DB, a Account) (Account, error) {
	return addAccount(db, a)
}
func addAccount(db *sql.DB, a Account) (Account, error) {
	jMetadata, err := json.Marshal(a.Metadata)
	if err != nil {
		log.Error(err)
	}
	ins := `insert into "accounts" ("key", "handle", "email", "score", "created_at", "updated_at", "flags", "metadata") 
	VALUES ($1, $2, $3, $4, $5, $6, $7::bit(8), $8)
	ON CONFLICT(email) DO UPDATE
		SET "score" = $4, "updated_at" = $6, "flags" = $7::bit(8), "metadata" = $8
	`

	if res, err := db.Exec(ins, a.Hash, a.Handle, a.Email, a.Score, a.CreatedAt, a.UpdatedAt, a.Flags, jMetadata); err == nil {
		if rows, _ := res.RowsAffected(); rows == 0 {
			return a, errors.Errorf("could not save account %s:%q", a.Handle, a.Hash)
		}
	} else {
		return a, err
	}

	return a, nil
}
