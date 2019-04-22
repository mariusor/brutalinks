package tests

import (
	"flag"
	"github.com/joho/godotenv"
	"github.com/mariusor/littr.go/app/cmd"
	"github.com/mariusor/littr.go/internal/log"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	var verbose int
	var clean bool
	flag.IntVar(&verbose, "verbose", 1, "the verbosity level for the output [0-6]")
	flag.BoolVar(&clean, "clean", true, "remove the test database at the end of the run")
	flag.Parse()
	configs := []string{
		".env",
		".env.test",
	}
	for _, f := range configs {
		godotenv.Overload(f)
	}

	createDB()
	defer func() {
		if clean {
			cmd.DestroyDB(r, o.User, o.Database)
		}
	}()
	go runAPP(log.Level(2 + verbose))

	os.Exit(m.Run())
}
