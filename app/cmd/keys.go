package cmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/log"
	"math/rand"
)

func GenSSHKey(handle string, seed int64, kType string) error {
	var err error
	if seed == 0 {
		err = errors.New("no seed value provided")
		if err != nil {
			return err
		}
	}
	loader := db.Config
	filter := app.LoadAccountsFilter{}
	if len(handle) != 0 {
		filter.Handle = []string{handle}
	} else {
		hashes := make([]string, 0)
		Logger.Info("No account handle, generating for all")

		sel := `select "key" from "accounts" where "id" != $1 AND "metadata"#>'{key}' is null;`
		rows, err := loader.DB.Query(sel, 0)
		if err != nil {
			return err
		}

		for rows.Next() {
			var hash string
			err := rows.Scan(&hash)
			if err != nil {
				return err
			}
			hashes = append(hashes, hash)
		}
		if len(hashes) == 0 {
			Logger.Warn("Nothing to do")
			return nil
		}
		filter.Key = hashes
		filter.MaxItems = len(hashes)
	}

	accts, err := loader.LoadAccounts(filter)
	if err != nil {
		return err
	}

	r := rand.New(rand.NewSource(seed))

	for _, acct := range accts {
		if acct.Metadata.Key != nil {
			Logger.Warnf("Existing Key for %s:%s//%d", acct.Handle, acct.Hash.String(), len(acct.Hash))
			continue
		}
		var pub, priv []byte
		if kType == "ecdsa" {
			var privKey *ecdsa.PrivateKey
			privKey, err = ecdsa.GenerateKey(elliptic.P224(), r)
			if err != nil {
				Logger.Error(err)
				continue
			}
			pub, err = x509.MarshalPKIXPublicKey(&privKey.PublicKey)
			if err != nil {
				Logger.Error(err)
				continue
			}
			priv, err = x509.MarshalECPrivateKey(privKey)
			if err != nil {
				Logger.Error(err)
				continue
			}
		} else {
			var privKey *rsa.PrivateKey
			privKey, err = rsa.GenerateKey(r, 2048)
			if err != nil {
				Logger.Error(err)
				continue
			}
			pub, err = x509.MarshalPKIXPublicKey(&privKey.PublicKey)
			if err != nil {
				Logger.Error(err)
				continue
			}
			priv, err = x509.MarshalPKCS8PrivateKey(privKey)
			if err != nil {
				Logger.Error(err)
				continue
			}
		}
		acct.Metadata.Key = &app.SSHKey{
			ID:      "id-" + kType,
			Public:  pub,
			Private: priv,
		}
		s, err := db.UpdateAccount(db.Config.DB, acct)
		if err != nil {
			Logger.Error(err)
			continue
		}
		log.WithFields(log.Fields{}).
			Infof("Updated Key for %s:%s//%d - %s:%s", s.Handle, s.Hash[0:8], len(s.Hash), s.Metadata.Key.ID, base64.StdEncoding.EncodeToString(s.Metadata.Key.Public))
	}
	return err
}
