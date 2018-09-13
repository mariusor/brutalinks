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
func main() {
	var handle string
	var seed int64
	flag.StringVar(&handle, "handle", "", "the content key to update votes for")
	flag.Int64Var(&seed, "seed", 0, "the seed used for the random number generator in key creation")
	flag.Parse()

	if seed == 0 {
		log.Error("no seed value provided")
		os.Exit(1)
		return
	}
	loader := models.Config
	filter := models.LoadAccountsFilter{}
	if len(handle) != 0 {
		filter.Handle = []string{handle}
	} else {
		err := errors.New("invalid handle, generating for all accounts")
		log.Error(err)

		sel := `select "key" from accounts where id != 0 AND metadata#>'{key}' is null;`
		rows, err := loader.DB.Query(sel)
		if err != nil {
			log.Error(err)
			os.Exit(1)
			return
		}

		for rows.Next() {
			var hash string
			err = rows.Scan(&hash)
			if err != nil {
				log.Error(err)
				os.Exit(1)
				return
			}
			filter.Key = append(filter.Key, hash)
		}
	}
	accts, err := loader.LoadAccounts(filter)
	if err != nil {
		log.Error(err)
		os.Exit(1)
		return
	}
	r := rand.New(rand.NewSource(seed))
	for _, acct := range accts {
		if privKey, err := ecdsa.GenerateKey(elliptic.P224(), r); err == nil {
			pub, errPub := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
			priv, errPrv := x509.MarshalECPrivateKey(privKey)
			if errPub == nil && errPrv == nil {
				acct.Metadata.Key = &models.SSHKey{
					Id:      "id-ecdsa",
					Public:  pub,
					Private: priv,
				}
				s, err := loader.SaveAccount(acct)
				if err != nil {
					log.Error(err)
					continue
				}
				log.WithFields(log.Fields{}).
					Infof("Updated Key for %s:%s - %s:%s", s.Handle, s.Hash[0:8], s.Metadata.Key.Id, base64.StdEncoding.EncodeToString(s.Metadata.Key.Public))

			} else {
				if errPub != nil {
					log.Error(errPub)
				}
				if errPrv != nil {
					log.Error(errPrv)
				}
			}
		} else {
			log.Error(err)
		}
	}
}
