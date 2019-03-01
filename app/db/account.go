package db

import (
	"fmt"
	"github.com/go-pg/pg"
	"github.com/mariusor/littr.go/app"
	"net/url"
	"strings"
	"time"

	"github.com/mariusor/littr.go/internal/errors"

	"github.com/mariusor/littr.go/internal/log"
)

// Logger is a local log instance
var Logger log.Logger

// Account represents the DB model that we are using
type Account struct {
	ID        int64               `sql:"id,auto"`
	Key       app.Key             `sql:"key,size(32)"`
	Email     string              `sql:"email"`
	Handle    string              `sql:"handle"`
	Score     int64               `sql:"score"`
	CreatedAt time.Time           `sql:"created_at"`
	UpdatedAt time.Time           `sql:"updated_at"`
	Flags     FlagBits            `sql:"flags"`
	Metadata  app.AccountMetadata `sql:"metadata"`
}

func UpdateAccount(db *pg.DB, a app.Account) (app.Account, error) {
	upd := `UPDATE "accounts" SET "score" = ?0, "updated_at" = ?1, "flags" = ?2::bit(8), "metadata" = ?3 where "key" ~* ?4;`

	if res, err := db.Exec(upd, a.Score, a.UpdatedAt, a.Flags, a.Metadata, a.Hash); err == nil {
		if rows := res.RowsAffected(); rows == 0 {
			return a, errors.Errorf("could not update account %s:%q", a.Handle, a.Hash)
		}
	} else {
		return a, errors.Annotatef(err, "DB query error")
	}

	return a, nil
}

func countAccounts(db *pg.DB, f app.LoadAccountsFilter) (uint, error) {
	wheres, whereValues := f.GetWhereClauses()
	var fullWhere string

	if len(wheres) == 0 {
		fullWhere = " true"
	} else if len(wheres) == 1 {
		fullWhere = fmt.Sprintf("%s", wheres[0])
	} else {
		fullWhere = fmt.Sprintf("(%s)", strings.Join(wheres, " AND "))
	}

	selC := fmt.Sprintf(`select count(*) from "accounts" where %s`, fullWhere)
	var count uint
	if _, err := db.Query(&count, selC, whereValues...); err != nil {
		return 0, errors.Annotatef(err, "DB query error")
	}
	return count, nil
}

func loadAccounts(db *pg.DB, f app.LoadAccountsFilter) (app.AccountCollection, error) {
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
	if _, err := db.Query(&agg, sel, whereValues...); err != nil {
		return nil, errors.Annotatef(err, "DB query error")
	}
	accounts := make(app.AccountCollection, len(agg))
	for k, acc := range agg {
		accounts[k] = acc.Model()
	}
	return accounts, nil
}

func saveAccount(db *pg.DB, a app.Account) (app.Account, error) {
	if len(a.Handle) == 0 {
		return a, errors.Errorf("invalid account to save")
	}

	acct := Account{
		Handle:   a.Handle,
		Score:    a.Score,
		Metadata: *a.Metadata,
	}
	if a.IsFederated() {
		acct.Key = app.GenKey([]byte(a.Metadata.ID))
	} else {
		acct.Key = app.GenKey([]byte(a.Handle))
		a.Metadata.ID = fmt.Sprintf("%s/self/following/%s", app.Instance.APIURL, url.PathEscape(acct.Key.String()))
	}

	if !a.CreatedAt.IsZero() {
		acct.CreatedAt = a.CreatedAt
	} else {
		acct.CreatedAt = time.Now()
	}
	if !a.UpdatedAt.IsZero() {
		acct.UpdatedAt = a.UpdatedAt
	} else {
		acct.UpdatedAt = time.Now()
	}

	acct.Flags.Scan(a.Flags)

	em := interface{}(nil)
	if len(a.Email) > 0 {
		em = interface{}(a.Email)
	}

	ins := `insert into "accounts" ("key", "handle", "email", "score", "created_at", "updated_at", "flags", "metadata")
	VALUES (?0, ?1, ?2, ?3, ?4, ?5, ?6::bit(8), ?7)
	ON CONFLICT("key") DO UPDATE SET "score" = ?3, "updated_at" = ?5, "flags" = ?6::bit(8), "metadata" = ?7;`

	if res, err := db.Exec(ins, acct.Key, acct.Handle, em, acct.Score, acct.CreatedAt, acct.UpdatedAt, a.Flags, acct.Metadata); err == nil {
		if rows := res.RowsAffected(); rows == 0 {
			return a, errors.Errorf("could not insert account %s:%q", acct.Handle, acct.Key)
		} else {
			Logger.Infof("%d", rows)
		}
	} else {
		return a, errors.Annotatef(err, "DB query error")
	}

	a.Hash = acct.Key.Hash()
	return a, nil
}
