package queue

import (
	"database/sql"
	"github.com/go-pg/pg"
	"github.com/go-ap/errors"
	"github.com/mariusor/littr.go/app"
)

type FetchQueue struct {
	Id   int     `sql:"id,auto"`
	Key  app.Key `sql:"key,size(32)"`
	Uri  string  `sql:"uri,varchar"`
	Done bool    `sql:"done,boolean"`
}

func (r Repository) AddToFetchQueue(uri string) (int64, error) {
	return addToFetchQueue(r.Backend.db, uri)
}

func addToFetchQueue(db *pg.DB, uri string) (int64, error) {
	q := `INSERT INTO "fetch_queue" ("uri", "done") VALUES($1, $2);`

	var res sql.Result
	var err error
	if _, err = db.Query(nil, q, uri, false); err != nil {
		return 0, errors.Annotatef(err, "unable to add URI to fetch queue")
	}
	return res.RowsAffected()
}
