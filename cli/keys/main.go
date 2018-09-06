package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"database/sql"
	"flag"
	"fmt"
	"math/rand"
	"os"

	"github.com/mariusor/littr.go/models"
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

	loader := models.Config
	acct, err := loader.LoadAccount(models.LoadAccountFilter{
		Handle: handle,
	})
	if err == nil {
		m := acct.Metadata

		if seed != 0 {
			if privKey, err := ecdsa.GenerateKey(elliptic.P224(), rand.New(rand.NewSource(seed))); err == nil {

				pub, errPub := x509.MarshalPKIXPublicKey(privKey.PublicKey)
				priv, errPrv := x509.MarshalECPrivateKey(privKey)
				if errPub == nil && errPrv == nil {
					m.Key = &models.SSHKey{
						Id:      "id-ecdsa",
						Public:  pub,
						Private: priv,
					}

					// TODO(marius): add the actual stuff
					//loader.SaveAccount()
				} else if errPub != nil {
					log.Error(errPub)
				} else {
					log.Error(errPrv)
				}
			} else {
				log.Error(err)
			}
		}
	} else {
		log.Error(err)
	}
}
