package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/mariusor/littr.go/app/models"

	_ "github.com/lib/pq"
)

var defaultSince, _ = time.ParseDuration("90h")
var db *sql.DB

func init() {
	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	var err error
	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)

	models.Config.DB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
	}

}

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

	var err error
	// recount all votes for content items
	var scores []models.Score
	if accounts {
		which := ""
		val := ""
		if handle != "" || key != "" {
			if len(handle) > 0 {
				which = "handle"
				val = handle
			} else {
				which = "key"
				val = key
			}
		}
		scores, err = models.LoadScoresForAccounts(since, which, val)
	} else if items {
		scores, err = models.LoadScoresForItems(since, key)
	}
	if err != nil {
		panic(err)
	}

	for _, score := range scores {
		var upd string
		if score.Type == models.ScoreItem {
			upd = `update "content_items" set score = $1 where id = $2;`
		} else {
			upd = `update "accounts" set score = $1 where id = $2;`
		}
		_, err := models.Config.DB.Exec(upd, score.Score, score.Id)
		if err != nil {
			panic(err)
		}
	}
}
