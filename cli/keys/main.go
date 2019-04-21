package main

import (
	"flag"
	"github.com/go-pg/pg"
	"github.com/mariusor/littr.go/app/cmd"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/internal/log"
)

func main() {
	var handle string
	var seed int64
	var kType string
	flag.StringVar(&handle, "handle", "", "the content key to update votes for")
	flag.StringVar(&kType, "type", "rsa", "key type to use: ecdsa, rsa")
	flag.Int64Var(&seed, "seed", 0, "the seed used for the random number generator in key creation")
	flag.Parse()

	cmd.Logger = log.Dev(log.TraceLevel)
	db.Config.DB = pg.Connect(cmd.PGConfigFromENV())

	cmd.E(cmd.GenSSHKey(handle, seed, kType))
}
