package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"

	"github.com/jmoiron/sqlx"

	"github.com/mariusor/littr.go/app/db"

	_ "github.com/lib/pq"
	"github.com/mariusor/littr.go/app/models"
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

	if seed == 0 {
		err := errors.New("no seed value provided")
		e(err)
	}
	loader := db.Config
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
		filter.MaxItems = len(hashes)
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
		var pub, priv []byte
		var err error
		if kType == "ecdsa" {
			var privKey *ecdsa.PrivateKey
			privKey, err = ecdsa.GenerateKey(elliptic.P224(), r)
			e(err)
			pub, err = x509.MarshalPKIXPublicKey(&privKey.PublicKey)
			if err != nil {
				log.Error(err)
				continue
			}
			priv, err = x509.MarshalECPrivateKey(privKey)
			if err != nil {
				log.Error(err)
				continue
			}
		} else {
			var privKey *rsa.PrivateKey
			privKey, err = rsa.GenerateKey(r, 2048)
			e(err)
			pub, err = x509.MarshalPKIXPublicKey(&privKey.PublicKey)
			if err != nil {
				log.Error(err)
				continue
			}
			priv, err = x509.MarshalPKCS8PrivateKey(privKey)
			if err != nil {
				log.Error(err)
				continue
			}
		}
		acct.Metadata.Key = &models.SSHKey{
			ID:      "id-" + kType,
			Public:  pub,
			Private: priv,
		}
		s, err := db.UpdateAccount(db.Config.DB, acct)
		if err != nil {
			log.Error(err)
			continue
		}
		log.WithFields(log.Fields{}).
			Infof("Updated Key for %s:%s//%d - %s:%s", s.Handle, s.Hash[0:8], len(s.Hash), s.Metadata.Key.ID, base64.StdEncoding.EncodeToString(s.Metadata.Key.Public))
	}
}
