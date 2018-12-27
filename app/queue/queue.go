package queue

import (
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/mariusor/littr.go/app"
)

type Backend int

const (
	Postgres = Backend(iota)
	Redis
)

type pg struct {
	db *sqlx.DB
}

func initPg(app app.Application) pg {
	if app.Config.DB.Port == "" {
		app.Config.DB.Port = "5432"
	}

	connStr := fmt.Sprintf("host=%s user=%s password=%s port=%s dbname=%s sslmode=disable",
		app.Config.DB.Host, app.Config.DB.User, app.Config.DB.Pw, app.Config.DB.Port, app.Config.DB.Name)

	q := pg{}
	if db, err := sqlx.Open("postgres", connStr); err == nil {
		q.db = db
	}
	return q
}

type Repository struct {
	Type Backend
	Backend pg
}

func New(app app.Application, typ Backend) Repository {
	return Repository{
		Type: typ,
		Backend: initPg(app),
	}
}

