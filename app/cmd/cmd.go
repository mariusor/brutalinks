package cmd

import (
	"github.com/go-pg/pg"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app/log"
	"os"
	"reflect"
)

var Logger log.Logger

func IsNil(c interface{}) bool {
	return c == nil || (reflect.ValueOf(c).Kind() == reflect.Ptr && reflect.ValueOf(c).IsNil())
}

func E(errs ...error) bool {
	if len(errs) == 0 {
		return true
	}
	result := true
	for _, e := range errs {
		if e == nil {
			continue
		}
		fields := make(log.Ctx)
		var msg string
		switch err := e.(type) {
		case *errors.Err:
			if IsNil(err.Underlying()) {
				continue
			}
			f, l := err.Location()
			if f != "" {
				fields["file"] = f
			}
			if l != 0 {
				fields["line"] = l
			}
			s := err.StackTrace()
			if len(s) > 0 {
				fields["trace"] = s
			}
			msg = err.Error()
		default:
			msg = err.Error()
		}
		Logger.WithContext(fields).Error(msg)
		result = false
	}

	return result
}

func PGConfigFromENV() *pg.Options {
	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")
	dbHost := os.Getenv("DB_HOST")

	return &pg.Options{
		User:     dbUser,
		Password: dbPw,
		Database: dbName,
		Addr: dbHost+":5432",
	}
}
