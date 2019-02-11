package main

import (
	"database/sql"
	"flag"
	"fmt"
	"github.com/mariusor/littr.go/app/cmd"
	"github.com/mariusor/littr.go/internal/log"
	"net"
	"os"
	"time"

	"github.com/gchaincl/dotsql"
	"github.com/juju/errors"
	_ "github.com/lib/pq"
)

func waitForDb(db *sql.DB, d time.Duration) (*sql.DB, error) {
	cnt := 0
	st := time.Now()
	for {
		if err := db.Ping(); err == nil {
			if cnt > 0 {
				fmt.Printf("\n")
			}
			return db, nil
		} else {
			if t, ok := err.(*net.OpError); ok {
				cnt++
				if cnt%10 == 0 {
					fmt.Printf(".")
				}
				if cnt%720 == 0 {
					fmt.Printf("\n")
				}
				time.Sleep(100 * time.Millisecond)
			} else {
				return db, t
			}
			if time.Since(st) > d {
				return db, errors.NotFoundf("No response for %d s, giving up.", d.Seconds())
			}
		}
		return db, nil
	}
	return db, nil
}

func dbConnection(dbHost string, dbUser string, dbPw string, dbName string) (*sql.DB, error) {
	fmt.Printf("Connecting to %s@%s//%s\n", dbUser, dbHost, dbName)
	if dbUser == "" && dbPw == "" {
		err := errors.Forbiddenf("missing user and/or pw")
		if !cmd.E(err) {
			os.Exit(1)
		}
	}

	var pw string
	if dbPw != "" {
		pw = fmt.Sprintf(" password=%s", dbPw)
	}
	connStr := fmt.Sprintf("host=%s user=%s%s dbname=%s sslmode=disable", dbHost, dbUser, pw, dbName)
	db, err := sql.Open("postgres", connStr)
	if err == nil {
		return waitForDb(db, time.Second*30)
	}
	return nil, err
}

func main() {
	var dbRootUser string
	var dbHost string
	var seed bool

	var err error

	cmd.Logger = log.Dev()

	flag.StringVar(&dbRootUser, "user", "", "the admin user for the database")
	flag.StringVar(&dbHost, "host", "", "the db host")
	flag.BoolVar(&seed, "seed", false, "seed database with data")
	flag.Parse()

	dbRootPw := os.Getenv("POSTGRES_PASSWORD")
	dbPw := os.Getenv("DB_PASSWORD")
	dbUser := os.Getenv("DB_USER")
	dbName := os.Getenv("DB_NAME")
	if dbHost == "" {
		dbHost = os.Getenv("DB_HOST")
	}
	if len(dbRootUser) == 0 {
		dbRootUser = "postgres"
	}
	dbRootName := "postgres"

	var dot *dotsql.DotSql
	rootDB, err := dbConnection(dbHost, dbRootUser, dbRootPw, dbRootName)
	if !cmd.E(errors.Annotate(err, "connection failed")) {
		os.Exit(1)
	}
	defer rootDB.Close()

	// create new role and db with root user in root database
	dot, err = dotsql.LoadFromFile("./db/create_role.sql")
	if !cmd.E(errors.Annotatef(err, "unable to load file")) {
		os.Exit(1)
	}
	s1, _ := dot.Raw("create-role-with-pass")
	role := fmt.Sprintf(s1, dbUser, "%s")
	_, err = rootDB.Exec(fmt.Sprintf(role, dbPw))
	if !cmd.E(errors.Annotatef(err, "query: %s", role)) {
		os.Exit(1)
	}

	s2, _ := dot.Raw("create-db-for-role")
	creatDb := fmt.Sprintf(s2, dbName, dbUser)
	_, err = rootDB.Exec(creatDb)
	if !cmd.E(errors.Annotatef(err, "query: %s", s2)) {
		os.Exit(1)
	}

	// root user, but our new created db
	db, err := dbConnection(dbHost, dbRootUser, dbRootPw, dbName)
	if !cmd.E(errors.Annotate(err, "connection failed")) {
		os.Exit(1)
	}
	defer db.Close()

	dot, err = dotsql.LoadFromFile("./db/extensions.sql")
	if !cmd.E(errors.Annotatef(err, "unable to load file")) {
		os.Exit(1)
	}
	_, err = dot.Exec(db, "extension-pgcrypto")
	s1, _ = dot.Raw("extension-pgcrypto")
	if !cmd.E(errors.Annotatef(err, "query: %s", s1)) {
		os.Exit(1)
	}
	_, err = dot.Exec(db, "extension-ltree")
	s2, _ = dot.Raw("extension-ltree")
	if !cmd.E(errors.Annotatef(err, "query: %s", s2)) {
		os.Exit(1)
	}

	// newly created user in newly created database
	db, err = dbConnection(dbHost, dbUser, dbPw, dbName)
	if !cmd.E(err) {
		os.Exit(1)
	}
	defer db.Close()
	dot, err = dotsql.LoadFromFile("./db/init.sql")
	if !cmd.E(errors.Annotatef(err, "unable to load file")) {
		os.Exit(1)
	}
	drop, _ := dot.Raw("drop-tables")
	_, err = db.Exec(fmt.Sprintf(drop))
	cmd.E(errors.Annotatef(err, "query: %s", drop))

	accounts, _ := dot.Raw("create-accounts")
	_, err = db.Exec(fmt.Sprintf(accounts))
	cmd.E(errors.Annotatef(err, "query: %s", accounts))

	items, _ := dot.Raw("create-items")
	_, err = db.Exec(fmt.Sprintf(items))
	cmd.E(errors.Annotatef(err, "query: %s", items))

	votes, _ := dot.Raw("create-votes")
	_, err = db.Exec(fmt.Sprintf(votes))
	cmd.E(errors.Annotatef(err, "query: %s", votes))

	instances, _ := dot.Raw("create-instances")
	_, err = db.Exec(fmt.Sprintf(instances))
	cmd.E(errors.Annotatef(err, "query: %s", instances))

	if seed {
		dot, err = dotsql.LoadFromFile("./db/seed.sql")
		if !cmd.E(errors.Annotatef(err, "unable to load file")) {
			os.Exit(1)
		}

		sysAcct, _ := dot.Raw("add-account-system")
		_, err = db.Exec(fmt.Sprintf(sysAcct))
		cmd.E(errors.Annotatef(err, "query: %s", sysAcct))

		anonAcct, _ := dot.Raw("add-account-anonymous")
		_, err = db.Exec(fmt.Sprintf(anonAcct))
		cmd.E(errors.Annotatef(err, "query: %s", anonAcct))

		itemAbout, _ := dot.Raw("add-item-about")
		_, err = db.Exec(fmt.Sprintf(itemAbout))
		cmd.E(errors.Annotatef(err, "query: %s", itemAbout))

		hostname := os.Getenv("HOSTNAME")
		localInst, _ := dot.Raw("add-local-instance")
		_, err = db.Exec(fmt.Sprintf(localInst, hostname, hostname))
		cmd.E(errors.Annotatef(err, "query: %s", localInst))
	}
}
