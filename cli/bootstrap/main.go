package main

import (
	"flag"
	"github.com/go-pg/pg"
	"github.com/mariusor/littr.go/app/cmd"
	"github.com/mariusor/littr.go/internal/log"
	"os"
)

func main() {
	var dbRootUser string
	var dbHost string
	var seed bool

	cmd.Logger = log.Dev()

	flag.StringVar(&dbRootUser, "user", "", "the admin user for the database")
	flag.StringVar(&dbHost, "host", "", "the db host")
	flag.BoolVar(&seed, "seed", false, "seed database with data")
	flag.Parse()

	dbRootPw := os.Getenv("POSTGRES_PASSWORD")
	if len(dbRootUser) == 0 {
		dbRootUser = "postgres"
	}
	dbRootName := "postgres"
	hostname := os.Getenv("HOSTNAME")

	o := cmd.PGConfigFromENV()
	r := &pg.Options{
		User:     dbRootUser,
		Password: dbRootPw,
		Database: dbRootName,
		Addr:     dbHost + ":5432",
	}

	cmd.E(cmd.CreateDatabase(o, r))
	cmd.E(cmd.BootstrapDB(o))
	if seed {
 		cmd.E(cmd.SeedDB(o, hostname))
	}
}
