package main

import (
	"flag"
	"github.com/go-pg/pg"
	"github.com/mariusor/littr.go/app/cmd"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/log"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	var handle string
	var seed int64
	var kType string
	flag.StringVar(&handle, "handle", "", "the content key to update votes for")
	flag.StringVar(&kType, "type", "rsa", "key type to use: ecdsa, rsa")
	flag.Int64Var(&seed, "seed", 0, "the seed used for the random number generator in key creation")
	flag.Parse()

	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	var err error

	db.Config.DB = pg.Connect(&pg.Options{
		User:     dbUser,
		Password: dbPw,
		Database: dbName,
	})
	cmd.Logger = log.Dev()
	if err != nil {
		cmd.Logger.Error(err.Error())
	}

	err = cmd.GenSSHKey(handle, seed, kType)
	cmd.E(err)
}
