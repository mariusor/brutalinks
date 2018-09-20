package db

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app/models"
)

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

func loadAccount(db *sqlx.DB, f models.LoadAccountsFilter) (models.Account, error) {
	return models.Account{}, errors.New("not implemented")
}

func loadAccounts(db *sqlx.DB, f models.LoadAccountsFilter) (models.AccountCollection, error) {
	return nil, errors.New("not implemented")
}

func saveAccount(db *sqlx.DB, a models.Account) (models.Account, error) {
	return a, errors.New("not implemented")
}
