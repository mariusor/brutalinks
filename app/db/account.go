package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app/models"

	log "github.com/sirupsen/logrus"
)

var Logger log.FieldLogger

type Account struct {
	Id        int64     `db:"id,auto"`
	Key       Key       `db:"key,size(64)"`
	Email     []byte    `db:"email"`
	Handle    string    `db:"handle"`
	Score     int64     `db:"score"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Flags     FlagBits  `db:"flags"`
	Metadata  Metadata  `db:"metadata"`
}

func loadAccounts(db *sqlx.DB, f models.LoadAccountsFilter) (models.AccountCollection, error) {
	wheres, whereValues := f.GetWhereClauses()

	var fullWhere string
	if len(wheres) > 0 {
		fullWhere = fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(wheres, " AND ")))
	} else {
		fullWhere = " true"
	}
	sel := fmt.Sprintf(`select 
		"id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags"
	from "accounts" where %s`, fullWhere)

	agg := make([]Account, 0)
	if err := db.Select(&agg, sel, whereValues...); err != nil {
		return nil, err
	}

	udb := db.Unsafe()
	if err := udb.Select(&agg, sel, whereValues...); err != nil {
		return nil, err
	}
	accounts := make(models.AccountCollection, len(agg))
	for k, acc := range agg {
		accounts[k] = acc.Model()
	}
	return accounts, nil
}

func saveAccount(db *sqlx.DB, a models.Account) (models.Account, error) {
	return a, errors.New("not implemented")
}

func UpdateAccount(db *sqlx.DB, a models.Account) (models.Account, error) {
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
		return a, err
	}

	return a, nil
}
