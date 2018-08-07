package main

import (
	"database/sql"
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"time"

	"github.com/mariusor/littr.go/models"

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

	// recount all votes for content items
	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Print(err)
	}

	var q string
	par := make([]interface{}, 0)
	par = append(par, interface{}(since.Hours()))

	if accounts {
		items = false
	}

	if items {
		if key == "" {
			q = `select "item_id", "key", max("content_items"."submitted_at"),
		sum(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
		sum(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
		from "votes" inner join "content_items" on "content_items"."id" = "item_id"
		where current_timestamp - "content_items"."submitted_at" < ($1 * INTERVAL '1 hour') group by "item_id", "key" order by "item_id";`
		} else {
			q = `select "item_id", "key", max("content_items"."submitted_at"),
		sum(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
		sum(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
		from "votes" inner join "content_items" on "content_items"."id" = "item_id"
		where current_timestamp - "content_items"."submitted_at" < ($1 * INTERVAL '1 hour') and "content_items"."key" ~* $2 group by "item_id", "key" order by "item_id";`
			par = append(par, interface{}(key))
		}
		log.Printf("Checking votes since %s", since)
		rows, err := db.Query(q, par...)
		if err != nil {
			panic(err)
		}
		for rows.Next() {
			var i, ups, downs int64
			var submitted time.Time
			var key []byte
			err = rows.Scan(&i, &key, &submitted, &ups, &downs)

			now := time.Now()
			reddit := int64(models.Reddit(ups, downs, now.Sub(submitted)) * models.ScoreMultiplier)
			wilson := int64(models.Wilson(ups, downs) * models.ScoreMultiplier)
			hacker := int64(models.Hacker(ups-downs, now.Sub(submitted)) * models.ScoreMultiplier)
			log.Printf("Votes[%s:%s]: UPS[%d] DOWNS[%d] - new score %d:%d:%d", key[0:8], now.Sub(submitted), ups, downs, reddit, wilson, hacker)
			score := wilson
			upd := `update "content_items" set score = $1 where id = $2;`
			_, err := db.Exec(upd, score, i)
			if err != nil {
				panic(err)
			}
		}
	}

	if accounts {
		if handle == "" && key == "" {
			q = `select "accounts"."id", "accounts"."handle", "accounts"."key", max("content_items"."submitted_at"),
       sum(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
       sum(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
from "votes"
       inner join "content_items" on "content_items"."id" = "item_id"
       inner join "accounts" on "content_items"."submitted_by" = "accounts"."id"
where current_timestamp - "content_items"."submitted_at" < ($1 * INTERVAL '1 hour') 
group by "accounts"."id", "accounts"."key" order by "accounts"."id";`
		} else {
			which := "key"
			if handle != "" && key == "" {
				which = "handle"
				par = append(par, interface{}(handle))
			} else {
				par = append(par, interface{}(key))
			}

			q = fmt.Sprintf(`select "accounts"."id", "accounts"."handle", "accounts"."key", max("content_items"."submitted_at"),
       sum(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
       sum(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
from "votes"
       inner join "content_items" on "content_items"."id" = "item_id"
       inner join "accounts" on "content_items"."submitted_by" = "accounts"."id"
where current_timestamp - "content_items"."submitted_at" < ($1 * INTERVAL '1 hour') 
and "content_items"."%s" ~* $2 group by "accounts"."id", "accounts"."key" order by "accounts"."id";`, which)
		}
		log.Printf("Checking votes since %s", since)
		rows, err := db.Query(q, par...)
		if err != nil {
			panic(err)
		}
		for rows.Next() {
			var i, ups, downs int64
			var submitted time.Time
			var key []byte
			var handle string
			err = rows.Scan(&i, &handle, &key, &submitted, &ups, &downs)

			now := time.Now()
			reddit := int64(models.Reddit(ups, downs, now.Sub(submitted)) * models.ScoreMultiplier)
			wilson := int64(models.Wilson(ups, downs) * models.ScoreMultiplier)
			hacker := int64(models.Hacker(ups-downs, now.Sub(submitted)) * models.ScoreMultiplier)
			log.Printf("Votes[%s]: UPS[%d] DOWNS[%d] - new score %d:%d:%d", handle, ups, downs, reddit, wilson, hacker)
			score := hacker
			upd := `update "accounts" set "score" = $1 where id = $2;`
			_, err := db.Exec(upd, score, i)
			if err != nil {
				panic(err)
			}
		}
	}
}
