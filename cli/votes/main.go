package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"models"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var defaultSince, _ = time.ParseDuration("90h")

func main() {
	var key string
	var since time.Duration
	flag.StringVar(&key, "key", "", "the content key to update votes for")
	flag.DurationVar(&since, "since", defaultSince, "the content key to update votes for")
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
	par = append(par, interface{}(since.Seconds()))

	if key == "" {
		q = `select "item_id", max("content_items"."submitted_at"),
		sum(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
		sum(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
		from "votes" inner join "content_items" on "content_items"."id" = "item_id"
		where current_timestamp - "content_items"."submitted_at" < $1 group by "item_id" order by "item_id";`
	} else {
		q = `select "item_id", max("content_items"."submitted_at"),
		sum(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
		sum(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
		from "votes" inner join "content_items" on "content_items"."id" = "item_id"
		where current_timestamp - "content_items"."submitted_at" < $1 and "content_items"."key" ~* $2 group by "item_id" order by "item_id";`
		par = append(par, interface{}(key))
	}
	rows, err := db.Query(q, par...)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var i, ups, downs int64
		var submitted time.Time
		err = rows.Scan(&i, &submitted, &ups, &downs)

		now := time.Now()
		reddit := int64(models.Reddit(ups, downs, now.Sub(submitted)) * models.ScoreMultiplier)
		wilson := int64(models.Wilson(ups, downs) * models.ScoreMultiplier)
		hacker := int64(models.Hacker(ups-downs, now.Sub(submitted)) * models.ScoreMultiplier)
		log.Printf("Votes[%d:%s]: UPS[%d] DOWNS[%d] - new score %d:%d:%d", i, now.Sub(submitted), ups, downs, reddit, wilson, hacker)
		score := wilson
		upd := `update "content_items" set score = $1 where id = $2;`
		_, err := db.Exec(upd, score, i)
		if err != nil {
			panic(err)
		}
	}
}
