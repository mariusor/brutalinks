package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/gchaincl/dotsql"
	"github.com/juju/errors"
	_ "github.com/lib/pq"
)

func dbConnection(dbHost string, dbUser string, dbPw string, dbName string) (*sql.DB, error) {
	if dbUser == "" && dbPw == "" {
		er(errors.NewErr("missing user and/or pw"))
	}

	var pw string
	if dbPw != "" {
		pw = fmt.Sprintf(" password=%s", dbPw)
	}
	connStr := fmt.Sprintf("host=%s user=%s%s dbname=%s sslmode=disable", dbHost, dbUser, pw, dbName)
	return sql.Open("postgres", connStr)
}

func er(err errors.Err) bool {
	if err.Underlying() != nil {
		fields := make(log.Fields)
		f, l := err.Location()
		if f != "" {
			fields["file"] = f
		}
		if l != 0 {
			fields["line"] = l
		}
		s := err.StackTrace()
		if len(s) > 0 {
			fields["trace"] = s
		}
		log.WithFields(fields).Errorf("%s", err.Message())
		return true
	}

	return false
}

func main() {
	var dbRootUser string
	var dbRootPw string
	var dbHost string
	var seed bool

	log.SetFormatter(&log.TextFormatter{
		DisableColors:          false,
		DisableLevelTruncation: true,
		ForceColors:            true,
	})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)

	flag.StringVar(&dbRootUser, "user", "", "the admin user for the database")
	flag.StringVar(&dbRootPw, "pw", "", "the admin pass for the database")
	flag.StringVar(&dbHost, "host", "", "the db host")
	flag.BoolVar(&seed, "seed", false, "seed database with data")
	flag.Parse()

	dbPw := os.Getenv("DB_PASSWORD")
	dbUser := os.Getenv("DB_USER")
	dbName := os.Getenv("DB_NAME")
	if dbHost == "" {
		dbHost = os.Getenv("DB_HOST")
	}

	var dot *dotsql.DotSql
	if len(dbRootUser) != 0 {
		dbRootName := "postgres"
		rootDB, err := dbConnection(dbHost, dbRootUser, dbRootPw, dbRootName)
		if er(errors.NewErrWithCause(err, "connection failed")) {
			os.Exit(1)
		}

		defer rootDB.Close()

		// create new role and db with root user in root database
		dot, err = dotsql.LoadFromFile("./db/create_role.sql")
		s1, _ := dot.Raw("create-role-with-pass")
		_, err = rootDB.Exec(fmt.Sprintf(s1, dbUser, strings.Trim(dbPw, "'")))
		er(errors.NewErrWithCause(err, "query: %s", s1))

		s2, _ := dot.Raw("create-db-for-role")
		_, err = rootDB.Exec(fmt.Sprintf(s2, dbName, dbUser))
		er(errors.NewErrWithCause(err, "query: %s", s2))

		// root user, but our new created db
		db, err := dbConnection(dbHost, dbRootUser, dbRootPw, dbName)
		if er(errors.NewErrWithCause(err, "connection failed")) {
			os.Exit(1)
		}
		defer db.Close()
		dot, err = dotsql.LoadFromFile("./db/extensions.sql")
		_, err = dot.Exec(db, "extension-pgcrypto")
		s1, _ = dot.Raw("extension-pgcrypto")
		er(errors.NewErrWithCause(err, "query: %s", s1))
		_, err = dot.Exec(db, "extension-ltree")
		s2, _ = dot.Raw("extension-ltree")
		er(errors.NewErrWithCause(err, "query: %s", s2))
	}

	// newly created user in newly created database
	db, err := dbConnection(dbHost, dbUser, dbPw, dbName)
	if er(errors.NewErrWithCause(err, "connection failed")) {
		os.Exit(1)
	}
	defer db.Close()
	// create new role and db with root user in root database
	dot, err = dotsql.LoadFromFile("./db/init.sql")
	er(errors.NewErrWithCause(err, "unable to load file"))
	drop, _ := dot.Raw("drop-tables")
	_, err = db.Exec(fmt.Sprintf(drop))
	er(errors.NewErrWithCause(err, "query: %s", drop))

	accounts, _ := dot.Raw("create-accounts")
	_, err = db.Exec(fmt.Sprintf(accounts))
	er(errors.NewErrWithCause(err, "query: %s", accounts))

	items, _ := dot.Raw("create-items")
	_, err = db.Exec(fmt.Sprintf(items))
	er(errors.NewErrWithCause(err, "query: %s", items))

	votes, _ := dot.Raw("create-votes")
	_, err = db.Exec(fmt.Sprintf(votes))
	er(errors.NewErrWithCause(err, "query: %s", votes))

	if seed {
		dot, err = dotsql.LoadFromFile("./db/seed.sql")
		er(errors.NewErrWithCause(err, "unable to load file"))

		sysAcct, _ := dot.Raw("add-account-system")
		_, err = db.Exec(fmt.Sprintf(sysAcct))
		er(errors.NewErrWithCause(err, "query: %s", sysAcct))

		anonAcct, _ := dot.Raw("add-account-anonymous")
		_, err = db.Exec(fmt.Sprintf(anonAcct))
		er(errors.NewErrWithCause(err, "query: %s", anonAcct))

		itemAbout, _ := dot.Raw("add-item-about")
		_, err = db.Exec(fmt.Sprintf(itemAbout))
		er(errors.NewErrWithCause(err, "query: %s", itemAbout))
	}
}
