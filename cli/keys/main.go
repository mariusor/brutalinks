package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"

	_ "github.com/lib/pq"
	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

func init() {
	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Print(err)
	}

	models.Config.DB = db
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
	flag.StringVar(&handle, "handle", "", "the content key to update votes for")
	flag.Int64Var(&seed, "seed", 0, "the seed used for the random number generator in key creation")
	flag.Parse()

	if seed == 0 {
		err := errors.New("no seed value provided")
		e(err)
	}
	loader := models.Config
	filter := models.LoadAccountsFilter{}
	if len(handle) != 0 {
		filter.Handle = []string{handle}
	} else {
		hashes := make([]string, 0)
		log.Info("No account handle, generating for all")

		sel := `select "key" from "accounts" where "id" != $1 AND "metadata"#>'{key}' is null;`
		rows, err := loader.DB.Query(sel, 0)
		e(err)

		for rows.Next() {
			var hash string
			err := rows.Scan(&hash)
			e(err)
			hashes = append(hashes, hash)
		}
		if len(hashes) == 0 {
			log.WithFields(log.Fields{}).Warn("Nothing to do")
			return
		}
		filter.Key = hashes
	}

	accts, err := loader.LoadAccounts(filter)
	e(err)

	r := rand.New(rand.NewSource(seed))

	for _, acct := range accts {
		if acct.Metadata.Key != nil {
			log.WithFields(log.Fields{}).
				Warnf("Existing Key for %s:%s//%d", acct.Handle, acct.Hash.String(), len(acct.Hash))
			continue
		}
		privKey, err := ecdsa.GenerateKey(elliptic.P224(), r)
		e(err)

		pub, errPub := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
		if errPub != nil {
			log.Error(errPub)
			continue
		}
		priv, errPrv := x509.MarshalECPrivateKey(privKey)
		if errPrv != nil {
			log.Error(errPrv)
			continue
		}
		acct.Metadata.Key = &models.SSHKey{
			Id:      "id-ecdsa",
			Public:  pub,
			Private: priv,
		}
		s, err := models.UpdateAccount(models.Config.DB, acct)
		if err != nil {
			log.Error(err)
			continue
		}
		log.WithFields(log.Fields{}).
			Infof("Updated Key for %s:%s//%d - %s:%s", s.Handle, s.Hash[0:8], len(s.Hash), s.Metadata.Key.Id, base64.StdEncoding.EncodeToString(s.Metadata.Key.Public))
	}
}
