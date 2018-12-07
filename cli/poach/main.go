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

var defaultSince, _ = time.ParseDuration("24h")
func main() {
	var url string
	var since time.Duration
	flag.StringVar(&url, "url", "", "the url of the feed to load")
	flag.DurationVar(&since, "since", defaultSince, "the content key to update votes for, default is 90h")
	flag.Parse()

	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	var err error
	cmd.Logger = log.Dev()
	db.Logger = cmd.Logger

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)
	if db.Config.DB, err = sqlx.Connect("postgres", connStr); err != nil {
		cmd.E(err)
		os.Exit(1)
	}

	err = cmd.PoachFeed(url, since)
	if !cmd.E(err) {
		os.Exit(1)
	}
}
