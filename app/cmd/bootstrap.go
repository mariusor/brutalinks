package cmd

import (
	"database/sql"
	"fmt"
	"github.com/gchaincl/dotsql"
	"github.com/go-pg/pg"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/internal/errors"
	"github.com/mmcloughlin/meow"
	"net"
	"os"
	"sort"
	"strconv"
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
	revOnDb :=fmt.Sprintf( "REVOKE CONNECT ON DATABASE %s FROM public;", dbName)
	if _, err = rootDB.Exec(revOnDb); err != nil {
		errs = append(errs, errors.Annotatef(err, "query: %s", revOnDb))
	}
	reassignOnDb := fmt.Sprintf("REASSIGN OWNED BY %s TO postgres;", dbName)
	if _, err = rootDB.Exec(reassignOnDb); err != nil {
		errs = append(errs, errors.Annotatef(err, "query: %s", reassignOnDb))
	}
	//disconnect := "DISCONNECT ALL;"
	//if _, err = rootDB.Exec(disconnect); err != nil {
	//	errs = append(errs, errors.Annotatef(err, "query: %s", disconnect))
	//}
	dropOwned :=fmt.Sprintf( "DROP OWNED BY %s CASCADE;", dbUser)  // needs to change db
	if _, err = rootDB.Exec(dropOwned); err != nil {
		errs = append(errs, errors.Annotatef(err, "query: %s", dropOwned))
	}
	dropDb := fmt.Sprintf( "DROP DATABASE IF EXISTS %s;", dbName)
	if _, err = rootDB.Exec(dropDb); err != nil {
		errs = append(errs, errors.Annotatef(err, "query: %s", dropDb))
	} else {
		dropRole := fmt.Sprintf("DROP ROLE IF EXISTS %s;", dbUser)
		if _, err = rootDB.Exec(dropRole); err != nil {
			errs = append(errs, errors.Annotatef(err, "query: %s", dropRole))
		}
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

func SeedTestData(o *pg.Options, seed map[string][][]interface{}) []error {
	dot, err := dotsql.LoadFromFile("./db/seed-parametrized.sql")
	if err != nil {
		return []error{errors.Annotatef(err, "unable to load file")}
	}
	db, err := dbConnection(o)
	if err != nil {
		return []error{errors.Annotatef(err, "unable to connect to DB")}
	}
	defer db.Close()

	keys := make([]string,0)
	for l := range seed {
		keys = append(keys, l)
	}
	sort.Strings(keys)
	errs := make([]error, 0)
	for _, l := range keys {
		name := "test-" + l
		sql, err := dot.Raw(name)
		if err != nil {
			errs = append(errs, err)
		}
		for _, item := range seed[l] {
			if _, err := db.Exec(sql, item...); err != nil {
				errs = append(errs, errors.Annotatef(err, "query: %s", sql))
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func SeedDB(o *pg.Options, hostname string) error {
	db, err := dbConnection(o)
	if err != nil {
		return errors.Annotatef(err, "unable to connect to DB")
	}
	defer db.Close()

	https, _ := strconv.ParseBool(os.Getenv("HTTPS"))
	scheme := "http"
	if https {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s", scheme, hostname)
	oauth2Key := os.Getenv("OAUTH2_KEY")
	if oauth2Key == "" {
		oauth2Key = fmt.Sprintf("%2x", meow.Checksum(app.RANDOM_SEED_SELECTED_BY_DICE_ROLL, []byte(url)))
	}
	oauth2Secret := os.Getenv("OAUTH2_SECRET")
	if oauth2Secret == "" {
		oauth2Secret = "yuh4ckm3?!"
	}
	data :=  map[string][][]interface{}{
		"accounts": {
			{
				// id
				interface{}(-1),
				// key
				interface{}("dc6f5f5bf55bc1073715c98c69fa7ca8"), // (meow.Checksum(app.RANDOM_SEED_SELECTED_BY_DICE_ROLL, []byte([]byte("system")))),
				// handle
				interface{}("system"),
				// email
				interface{}("system@localhost"),
				// metadata
				interface{}("{}"),
			},
			{
				// id
				interface{}(0),
				// key
				interface{}("eacff9ddf379bd9fc8274c5a9f4cae08"), // (meow.Checksum(app.RANDOM_SEED_SELECTED_BY_DICE_ROLL, []byte("anonymous"))),
				// handle
				interface{}("anonymous"),
				// email
				interface{}("anonymous@localhost"),
				// metadata
				interface{}("{}"),
			},
		},
		"items": {
			{
				// key
				interface{}("162edb32c80d0e6dd3114fbb59d6273b"),
				// mime_type
				interface{}("text/html"),
				// title
				interface{}("about littr.me"),
				// data
				interface{}(`<p>This is a new attempt at the social news aggregator paradigm.<br/> 
				It's based on the ActivityPub web specification and as such tries to leverage federation to prevent some of the pitfalls found in similar existing communities.</p>`),
				// submitted_by
				interface{}(-1),
				// path
				interface{}(nil),
				// metadata
				interface{}("{}"),
			},
		},
		"votes": {},
		"instances": {
			{
				// id
				interface{}(0),
				// name
				interface{}("Local instance - DEV"),
				// description
				interface{}("Link aggregator inspired by Reddit and HackerNews using ActivityPub federation."),
				// url
				interface{}(url),
				// inbox
				interface{}("/api/self/inbox"),
				// metadata
				interface{}("{}"),
			},
			{
				// id
				interface{}(1),
				// name
				interface{}("littr.me"),
				// description
				interface{}(""),
				// url
				interface{}("https://littr.me"),
				// inbox
				interface{}("/api/self/inbox"),
				// metadata
				interface{}("{}"),
			},
		},
		"oauth-clients": {
			// TODO(marius): should we need to add an entry for littr.me also ?
			{
				// id - hashed hostname
				interface{}(oauth2Key),
				// secret - local one
				interface{}(oauth2Secret),
				// extra
				interface{}(nil),
				// redirect_uri
				interface{}(fmt.Sprintf("%s/auth/local/callback", url)), // this should point to a frontend uri that can handle oauth
			},
		},
	}

	SeedTestData(o, data)
	return nil
}

func CreateDatabase(o *pg.Options, r *pg.Options) error {
	{
		rootDB, err := dbConnection(r)
		if err != nil {
			return errors.Annotate(err, "connection failed")
		}
		defer rootDB.Close()
		// create new role and db with root user in root database
		dot, err := dotsql.LoadFromFile("./db/create_role.sql")
		if err != nil {
			return errors.Annotatef(err, "unable to load file")
		}
		s1, _ := dot.Raw("create-role-with-pass")
		role := fmt.Sprintf(s1, o.User, "%s")
		if _, err := rootDB.Exec(fmt.Sprintf(role, o.Password)); err != nil {
			if pe, ok := err.(*pq.Error); !ok || pe.Code != "42710" {
				return errors.Annotatef(err, "query: %s", role)
			}
		}

		s2, _ := dot.Raw("create-db-for-role")
		creatDb := fmt.Sprintf(s2, o.Database, o.User)
		if _, err := rootDB.Exec(creatDb); err != nil {
			if pe, ok := err.(*pq.Error); !ok || pe.Code != "42P04" {
				return errors.Annotatef(err, "query: %s", s2)
			}
		}
	}
	{
		// root user, but our new created db
		r.Database = o.Database
		db, err := dbConnection(r)
		if err != nil {
			return errors.Annotate(err, "connection failed")
		}
		defer db.Close()
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
	_, _ = db.Exec(drop)

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

	types, _ := dot.Raw("create-activitypub-types-enum")
	if _, err = db.Exec(types); err != nil {
		if pe, ok := err.(*pq.Error); !ok && pe.Code != "42710" {
			return errors.Annotatef(err, "query: %s", types)
		}
	}

	objects, _ := dot.Raw("create-activitypub-objects")
	if _, err = db.Exec(objects); err != nil {
		return errors.Annotatef(err, "query: %s", objects)
	}

	oauth, _ := dot.Raw("create-oauth-storage")
	if _, err = db.Exec(oauth); err != nil {
		return errors.Annotatef(err, "queries: %s", oauth)
	}

	if false {
		actors, _ := dot.Raw("create-activitypub-actors")
		if _, err = db.Exec(actors); err != nil {
			return errors.Annotatef(err, "query: %s", actors)
		}
		activities, _ := dot.Raw("create-activitypub-activities")
		if _, err = db.Exec(activities); err != nil {
			return errors.Annotatef(err, "query: %s", activities)
		}
	}

	return nil
}
