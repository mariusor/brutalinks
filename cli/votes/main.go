package main

import (
	"flag"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/mariusor/littr.go/app/cmd"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var defaultSince, _ = time.ParseDuration("90h")

func main() {
	var key string
	var handle string
	var since time.Duration
	var items bool
	var accounts bool
	flag.StringVar(&handle, "handle", "", "the content key to update votes for, implies -accounts")
	flag.StringVar(&key, "key", "", "the content key to update votes for")
	flag.BoolVar(&items, "items", true, "update scores for items")
	flag.BoolVar(&accounts, "accounts", false, "update scores for account")
	flag.DurationVar(&since, "since", defaultSince, "the content key to update votes for, default is 90h")
	flag.Parse()

	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	var err error
	cmd.Logger = log.Dev()
	db.Logger = cmd.Logger
	if err != nil {
		cmd.Logger.Error(err.Error())
	}

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)

	db.Config.DB, err = sqlx.Connect("postgres", connStr)
	if err != nil {
		cmd.Logger.Error(err.Error())
	}

	err = cmd.UpdateScores(key, handle, since, items, accounts)
	cmd.E(err)
}
