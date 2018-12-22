package cmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/log"
	"math/rand"
)

func accountNeedsKeyWithLog(acct app.Account, logger log.Logger ) bool {
	if len(acct.Metadata.ID) > 0 {
		logger.WithContext(log.Ctx{
			"handle": acct.Handle,
			"hash":   acct.Hash.String(),
		}).Infof("Federated account, skipping")
		return false
	}
	if acct.Metadata.Key != nil {
		logger.WithContext(log.Ctx{
			"handle": acct.Handle,
			"hash":   acct.Hash.String(),
		}).Infof("Existing Key")
		return false
	}
	return true
}

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
		if !accountNeedsKeyWithLog(acct, Logger) {
			continue
		}
		var pub, priv []byte
		if kType == "ecdsa" {
			var privKey *ecdsa.PrivateKey
			privKey, err = ecdsa.GenerateKey(elliptic.P224(), r)
			if err != nil {
				Logger.Error(err.Error())
				continue
			}
			pub, err = x509.MarshalPKIXPublicKey(&privKey.PublicKey)
			if err != nil {
				Logger.Error(err.Error())
				continue
			}
			priv, err = x509.MarshalECPrivateKey(privKey)
			if err != nil {
				Logger.Error(err.Error())
				continue
			}
		} else {
			var privKey *rsa.PrivateKey
			if privKey, err = rsa.GenerateKey(r, 2048); err != nil {
				Logger.Error(err.Error())
				continue
			}
			pub, err = x509.MarshalPKIXPublicKey(&privKey.PublicKey)
			if err != nil {
				Logger.Error(err.Error())
				continue
			}
			priv, err = x509.MarshalPKCS8PrivateKey(privKey)
			if err != nil {
				Logger.Error(err.Error())
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
			Logger.Error(err.Error())
			continue
		}
		ctx := log.Ctx{
			"handle": acct.Handle,
			"hash":   acct.Hash.String(),
		}
		if len(s.Metadata.Key.Public) > 0 {
			enc := base64.StdEncoding.EncodeToString(acct.Metadata.Key.Public)
			ctx["key-id"] = acct.Metadata.Key.ID
			ctx["pub"] = fmt.Sprintf("%s...", enc[0:10])
		}
		Logger.WithContext(ctx).Infof("Updated Key")
	}
	return err
}
