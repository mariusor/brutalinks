package main

import (
	"flag"
	"fmt"
	"github.com/mariusor/littr.go/app/cli"
	"os"

	"github.com/jmoiron/sqlx"

	"github.com/mariusor/littr.go/app/db"

	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

func init() {
	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	var err error
	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)
	db.Config.DB, err = sqlx.Open("postgres", connStr)
	if err != nil {
		log.Print(err)
	}
}

func e(err error) {
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
}

func main() {
	var handle string
	var seed int64
	var kType string
	flag.StringVar(&handle, "handle", "", "the content key to update votes for")
	flag.StringVar(&kType, "type", "rsa", "key type to use: ecdsa, rsa")
	flag.Int64Var(&seed, "seed", 0, "the seed used for the random number generator in key creation")
	flag.Parse()

	err := cli.GenSSHKey(handle, seed, kType)
	e(err)
}
