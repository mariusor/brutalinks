package queue

import (
	"fmt"
	"github.com/go-pg/pg"
	"github.com/mariusor/littr.go/app"
)

type Backend int

const (
	Postgres = Backend(iota)
	Redis
)

type pgQueue struct {
	db *pg.DB
}

func initPg(app app.Application) pgQueue {
	if app.Config.DB.Port == "" {
		app.Config.DB.Port = "5432"
	}

	db := pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", app.Config.DB.Host, app.Config.DB.Port),
		User:     app.Config.DB.User,
		Password: app.Config.DB.Pw,
		Database: app.Config.DB.Name,
	})
	return pgQueue{db: db}
}

type Repository struct {
	Type    Backend
	Backend pgQueue
}

func New(app app.Application, typ Backend) Repository {
	return Repository{
		Type:    typ,
		Backend: initPg(app),
	}
}
