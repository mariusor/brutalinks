package queue

import (
	"database/sql"
	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app"
)

type FetchQueue struct {
	Id   int     `db:"id,auto"`
	Key  app.Key `db:"key,size(32)"`
	Uri  string  `db:"uri,varchar"`
	Done bool    `db:"done,boolean"`
}

func (r Repository) AddToFetchQueue(uri string) (int64, error) {
	return addToFetchQueue(r.Backend.db, uri)
}

func addToFetchQueue(db *sqlx.DB, uri string) (int64, error) {
	q := `INSERT INTO "fetch_queue" ("uri", "done") VALUES($1, $2);`

	var res sql.Result
	var err error
	if res, err = db.Exec(q, uri, false); err != nil {
		return 0, errors.Annotatef(err, "unable to add URI to fetch queue")
	}
	return res.RowsAffected()
}
