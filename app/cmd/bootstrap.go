package cmd

import (
	"database/sql"
	"fmt"
	"github.com/gchaincl/dotsql"
	"github.com/go-pg/pg"
	_ "github.com/lib/pq"
	"github.com/mariusor/littr.go/internal/errors"
	"net"
	"strings"
	"time"
)

func waitForDb(db *sql.DB, d time.Duration) (*sql.DB, error) {
	cnt := 0
	st := time.Now()
	for {
		if err := db.Ping(); err == nil {
			//if cnt > 0 {
			//	fmt.Printf("\n")
			//}
			return db, nil
		} else {
			if _, ok := err.(*net.OpError); ok {
				cnt++
				//if cnt%10 == 0 {
				//	fmt.Printf(".")
				//}
				//if cnt%720 == 0 {
				//	fmt.Printf("\n")
				//}
				time.Sleep(100 * time.Millisecond)
			} else {
				return db, err
			}
			if time.Since(st) > d {
				return db, errors.NotFoundf("No response for %d s, giving up.", d.Seconds())
			}
		}
		return db, nil
	}
	return db, nil
}

func dbConnection(o *pg.Options) (*sql.DB, error) {
	//fmt.Printf("Connecting to %s@%s//%s\n", o.User, o.Addr, o.Database)
	if o.User == "" && o.Password == "" {
		return nil, errors.Forbiddenf("missing user and/or pw")
	}
	host := o.Addr
	port := " port=5432"
	if strings.Contains(o.Addr, ":") {
		parts := strings.Split(o.Addr, ":")
		if len(parts[0]) == 0 {
			host = "127.0.0.1"
		} else {
			host = parts[0]
		}
		if len(parts) == 2 {
			if len(parts[1]) > 0 {
				port = fmt.Sprintf(" port=%s", parts[1])
			}
		}
	}

	var pw string
	if o.Password != "" {
		pw = fmt.Sprintf(" password=%s", o.Password)
	}
	connStr := fmt.Sprintf("host=%s%s user=%s%s dbname=%s sslmode=disable", host, port, o.User, pw, o.Database)
	db, err := sql.Open("postgres", connStr)
	if err == nil {
		return waitForDb(db, time.Second*30)
	}
	return nil, err
}

func DestroyDB(r *pg.Options, dbUser, dbName string) []error {
	rootDB, err := dbConnection(r)
	if err != nil {
		return []error{errors.Annotate(err, "connection failed")}
	}
	defer rootDB.Close()

	var errs = make([]error, 0)
	revOnDb := "REVOKE CONNECT ON DATABASE %s FROM public;"
	if _, err = rootDB.Exec(fmt.Sprintf(revOnDb, dbName)); err != nil {
		errs = append(errs, errors.Annotatef(err, "query: %s", revOnDb))
	}
	reassignOnDb := "REASSIGN OWNED BY %s TO postgres;"
	if _, err = rootDB.Exec(fmt.Sprintf(reassignOnDb, dbName)); err != nil {
		errs = append(errs, errors.Annotatef(err, "query: %s", reassignOnDb))
	}
	dropOwned := "DROP OWNED BY %s CASCADE;" // needs to change db
	if _, err = rootDB.Exec(fmt.Sprintf(dropOwned, dbUser)); err != nil {
		errs = append(errs, errors.Annotatef(err, "query: %s", dropOwned))
	}
	dropDb := "DROP DATABASE IF EXISTS %s;"
	if _, err = rootDB.Exec(fmt.Sprintf(dropDb, dbName)); err != nil {
		errs = append(errs, errors.Annotatef(err, "query: %s", dropDb))
	}
	dropRole := "DROP ROLE IF EXISTS %s;"
	if _, err = rootDB.Exec(fmt.Sprintf(dropRole, dbUser)); err != nil {
		errs = append(errs, errors.Annotatef(err, "query: %s", dropRole))
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func CleanDB(o *pg.Options) error {
	db, err := dbConnection(o)
	if err != nil {
		return err
	}
	defer db.Close()

	var dot *dotsql.DotSql
	dot, err = dotsql.LoadFromFile("./db/init.sql")
	if err != nil {
		return errors.Annotatef(err, "unable to load file")
	}
	truncate, err := dot.Raw("truncate-tables")
	if err != nil {
		return errors.Annotatef(err, "unable to load query: truncate-tables")
	}
	if _, err = db.Exec(truncate); err != nil {
		return errors.Annotatef(err, "query: %s", truncate)
	}
	return nil
}

func SeedTestData(o *pg.Options, seed map[string][]interface{}) error {
	dot, err := dotsql.LoadFromFile("./db/seed-test.sql")
	if err != nil {
		return errors.Annotatef(err, "unable to load file")
	}
	db, err := dbConnection(o)
	if err != nil {
		return errors.Annotatef(err, "unable to connect to DB")
	}
	defer db.Close()

	for l, data := range seed {
		sql, err := dot.Raw("test-" + l)
		if err != nil {
			return err
		}
		if _, err := db.Exec(sql, data...); err != nil {
			return errors.Annotatef(err, "query: %s", sql)
		}
	}

	return nil
}

func SeedDB(o *pg.Options, hostname string) error {
	dot, err := dotsql.LoadFromFile("./db/seed.sql")
	if err != nil {
		return errors.Annotatef(err, "unable to load file")
	}
	db, err := dbConnection(o)
	if err != nil {
		return errors.Annotatef(err, "unable to connect to DB")
	}
	defer db.Close()

	sysAcct, _ := dot.Raw("add-account-system")
	if _, err := db.Exec(fmt.Sprintf(sysAcct)); err != nil {
		return errors.Annotatef(err, "query: %s", sysAcct)
	}
	anonAcct, _ := dot.Raw("add-account-anonymous")
	if _, err := db.Exec(fmt.Sprintf(anonAcct)); err != nil {
		return errors.Annotatef(err, "query: %s", anonAcct)
	}
	itemAbout, _ := dot.Raw("add-item-about")
	if _, err := db.Exec(fmt.Sprintf(itemAbout)); err != nil {
		return errors.Annotatef(err, "query: %s", itemAbout)
	}
	localInst, _ := dot.Raw("add-local-instance")
	if _, err := db.Exec(fmt.Sprintf(localInst, hostname, hostname)); err != nil {
		return errors.Annotatef(err, "query: %s", localInst)
	}
	return nil
}

func CreateDatabase(o *pg.Options, r *pg.Options) error {
	rootDB, err := dbConnection(r)
	if err != nil {
		return errors.Annotate(err, "connection failed")
	}
	defer rootDB.Close()
	{
		// create new role and db with root user in root database
		dot, err := dotsql.LoadFromFile("./db/create_role.sql")
		if err != nil {
			return errors.Annotatef(err, "unable to load file")
		}
		s1, _ := dot.Raw("create-role-with-pass")
		role := fmt.Sprintf(s1, o.User, "%s")
		if _, err := rootDB.Exec(fmt.Sprintf(role, o.Password)); err != nil {
			return errors.Annotatef(err, "query: %s", role)
		}

		s2, _ := dot.Raw("create-db-for-role")
		creatDb := fmt.Sprintf(s2, o.Database, o.User)
		if _, err := rootDB.Exec(creatDb); err != nil {
			return errors.Annotatef(err, "query: %s", s2)
		}
	}
	// root user, but our new created db
	r.Database = o.Database
	db, err := dbConnection(r)
	if err != nil {
		return errors.Annotate(err, "connection failed")
	}
	defer db.Close()
	{
		dot, err := dotsql.LoadFromFile("./db/extensions.sql")
		if err != nil {
			return errors.Annotatef(err, "unable to load file")
		}
		if _, err := dot.Exec(db, "extension-pgcrypto"); err != nil {
			s1, _ := dot.Raw("extension-pgcrypto")
			return errors.Annotatef(err, "query: %s", s1)
		}
		if _, err = dot.Exec(db, "extension-ltree"); err != nil {
			s2, _ := dot.Raw("extension-ltree")
			return errors.Annotatef(err, "query: %s", s2)
		}
	}
	return nil
}

func BootstrapDB(o *pg.Options) error {
	// newly created user in newly created database
	db, err := dbConnection(o)
	if err != nil {
		return err
	}
	defer db.Close()
	dot, err := dotsql.LoadFromFile("./db/init.sql")
	if err != nil {
		return errors.Annotatef(err, "unable to load file")
	}
	drop, _ := dot.Raw("drop-tables")
	if _, err = db.Exec(drop); err != nil {
		return errors.Annotatef(err, "query: %s", drop)
	}

	accounts, _ := dot.Raw("create-accounts")
	if _, err = db.Exec(accounts); err != nil {
		return errors.Annotatef(err, "query: %s", accounts)
	}

	items, _ := dot.Raw("create-items")
	if _, err = db.Exec(items); err != nil {
		return errors.Annotatef(err, "query: %s", items)
	}

	votes, _ := dot.Raw("create-votes")
	if _, err = db.Exec(votes); err != nil {
		return errors.Annotatef(err, "query: %s", votes)
	}

	instances, _ := dot.Raw("create-instances")
	if _, err = db.Exec(instances); err != nil {
		return errors.Annotatef(err, "query: %s", instances)
	}

	return nil
}
