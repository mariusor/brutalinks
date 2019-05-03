package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/go-pg/pg"
	_ "github.com/joho/godotenv/autoload"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/cmd"
	"github.com/mariusor/littr.go/internal/log"
	"os"
)

func getTestAccount(hostname string) []interface{} {
	const testActorHash = "f00f00f00f00f00f00f00f00f00f6667"
	const prv = "MIHDAgEAMA0GCSqGSIb3DQEBAQUABIGuMIGrAgEAAiEArZw0fx8IYdu7Z3TLW9csFwP1j90IFs43mrGq6u+hc9ECAwEAAQIgBGBPonSxzWWwj6cOCT6fSdqTsi9iLU/oUQQ7R4sAZlECEQDEGQ3cCXkYx4Nn4YDMeDwPAhEA4qSaUiOzFKSiq62OWvTyHwIRAKiQXNCLOBQr1HIkbsHUjNMCEBIgzF8pj9dk28YTmcFYuk0CEQCLwespMQNDWxjh+03J4LgU"
	const pub = "MDwwDQYJKoZIhvcNAQEBBQADKwAwKAIhAK2cNH8fCGHbu2d0y1vXLBcD9Y/dCBbON5qxqurvoXPRAgMBAAE="
	var meta = app.AccountMetadata{
		ID: fmt.Sprintf("%s/api/self/following/%s", hostname, testActorHash),
		Key: &app.SSHKey{
			ID:      fmt.Sprintf("%s/api/self/following/%s#main-key", hostname, testActorHash),
			Public:  []byte(pub),
			Private: []byte(prv),
		},
	}
	var jm, _ = json.Marshal(meta)
	return []interface{}{
		interface{}(666),
		interface{}(testActorHash),
		interface{}("johndoe"),
		interface{}(fmt.Sprintf("jd@%s", hostname)),
		interface{}(string(jm)),
	}
}

func main() {
	var dbRootUser, dbHost string
	var seed, testing, overwrite bool

	cmd.Logger = log.Dev(log.TraceLevel)

	flag.StringVar(&dbRootUser, "user", "", "the admin user for the database")
	flag.StringVar(&dbHost, "host", "", "the db host")
	flag.BoolVar(&seed, "seed", false, "seed database with data")
	flag.BoolVar(&testing, "testing", false, "seed database with testing data")
	flag.BoolVar(&overwrite, "overwrite", false, "destroy database if exists and recreate")
	flag.Parse()

	dbRootPw := os.Getenv("POSTGRES_PASSWORD")
	if len(dbRootUser) == 0 {
		dbRootUser = "postgres"
	}
	if dbHost == "" {
		dbHost = os.Getenv("DB_HOST")
	}
	dbRootName := "postgres"
	hostname := os.Getenv("HOSTNAME")
	oauthURL := os.Getenv("OAUTH2_URL")

	o := cmd.PGConfigFromENV()
	r := &pg.Options{
		User:     dbRootUser,
		Password: dbRootPw,
		Database: dbRootName,
		Addr:     dbHost + ":5432",
	}

	checkDb := cmd.CreateDatabase(o, r, overwrite)
	if checkDb == cmd.ErrDbExists {
		cmd.Logger.Warnf("WTF: %s", checkDb)
		if !overwrite {
			cmd.Logger.Infof("Exiting")
			return
		}
	}
	cmd.E(cmd.BootstrapDB(o))
	if seed {
		cmd.SeedDB(o, hostname, oauthURL)
	}
	if testing {
		var data = map[string][][]interface{}{
			"accounts": {
				getTestAccount(hostname),
			},
		}

		cmd.E(cmd.SeedTestData(o, data)...)
	}
}
