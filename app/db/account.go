package db

import (
	"encoding/json"
	"fmt"
	"github.com/mariusor/littr.go/app"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"

	"github.com/mariusor/littr.go/app/log"
)

// Logger is a local log instance
var Logger log.Logger

// Account represents the db model that we are using
type Account struct {
	Id        int64     `db:"id,auto"`
	Key       app.Key   `db:"key,size(32)"`
	Email     []byte    `db:"email"`
	Handle    string    `db:"handle"`
	Score     int64     `db:"score"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Flags     FlagBits  `db:"flags"`
	Metadata  Metadata  `db:"metadata"`
}

func UpdateAccount(db *sqlx.DB, a app.Account) (app.Account, error) {
	jMetadata, err := json.Marshal(a.Metadata)
	if err != nil {
		return a, err
	}
	upd := `UPDATE "accounts" SET "score" = $1, "updated_at" = $2, "flags" = $3::bit(8), "metadata" = $4 where "key" ~* $5;`

	if res, err := db.Exec(upd, a.Score, a.UpdatedAt, a.Flags, jMetadata, a.Hash); err == nil {
		if rows, _ := res.RowsAffected(); rows == 0 {
			return a, errors.Errorf("could not update account %s:%q", a.Handle, a.Hash)
		}
	} else {
		return a, errors.Annotatef(err, "db query error")
	}

	return a, nil
}

func loadAccounts(db *sqlx.DB, f app.LoadAccountsFilter) (app.AccountCollection, error) {
	wheres, whereValues := f.GetWhereClauses()
	var fullWhere string

	if len(wheres) == 0 {
		fullWhere = " true"
	} else if len(wheres) == 1 {
		fullWhere = fmt.Sprintf("%s", wheres[0])
	} else {
		fullWhere = fmt.Sprintf("(%s)", strings.Join(wheres, " AND "))
	}
	var offset string
	if f.Page > 0 {
		offset = fmt.Sprintf(" OFFSET %d", f.MaxItems*f.Page)
	}

	sel := fmt.Sprintf(`select 
		"id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags"
	from "accounts" where %s LIMIT %d%s`, fullWhere, f.MaxItems, offset)

	type AccountCollection []Account
	agg := make(AccountCollection, 0)
	if err := db.Select(&agg, sel, whereValues...); err != nil {
		return nil, errors.Annotatef(err, "db query error")
	}
	accounts := make(app.AccountCollection, len(agg))
	for k, acc := range agg {
		accounts[k] = acc.Model()
	}
	return accounts, nil
}

func saveAccount(db *sqlx.DB, a app.Account) (app.Account, error) {
	jMetadata, err := json.Marshal(a.Metadata)
	if err != nil {
		Logger.Error(err.Error())
	}
	acct := Account{
		Handle: a.Handle,
		Key: app.GenKey([]byte(a.Handle)),
		Score: a.Score,
		Metadata: jMetadata,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
	acct.Flags.Scan(a.Flags)

	em := interface{}(nil)
	if len(a.Email) > 0 {
		em = interface{}(a.Email)
	}

	ins := `insert into "accounts" ("key", "handle", "email", "score", "created_at", "updated_at", "flags", "metadata") 	
	VALUES ($1, $2, $3, $4, $5, $6, $7::bit(8), $8)	
	ON CONFLICT("key") DO UPDATE	
		SET "score" = $4, "updated_at" = $6, "flags" = $7::bit(8), "metadata" = $8	
	`
	if res, err := db.Exec(ins, acct.Key, acct.Handle, em, acct.Score, acct.CreatedAt, acct.UpdatedAt, a.Flags, acct.Metadata); err == nil {
		if rows, _ := res.RowsAffected(); rows == 0 {
			return a, errors.Errorf("could not insert account %s:%q", acct.Handle, acct.Key)
		}
	} else {
		return a, errors.Annotatef(err, "db query error")
	}

	a.Hash = acct.Key.Hash()
	return a, nil
}
