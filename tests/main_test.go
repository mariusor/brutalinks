package tests

import (
	"github.com/mariusor/littr.go/app/cmd"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	createDB()
	defer cmd.DestroyDB(r, o.User, o.Database)

	go runAPP()

	os.Exit(m.Run())
}
