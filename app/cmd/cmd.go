package cmd

import (
	"github.com/go-ap/errors"
	"github.com/go-pg/pg"
	"github.com/mariusor/littr.go/internal/log"
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
			if IsNil(err.Unwrap()) {
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
			wrapMessages := make([]string, 0)
			wrapped := err.Unwrap()
			for {
				if wrapped == nil {
					break
				}
				wrapMessages = append(wrapMessages, wrapped.Error())
				if werr, ok := wrapped.(*errors.Err); ok {
					wrapped = werr.Unwrap()
				} else {
					break
				}
			}
			if len(wrapMessages) > 0 {
				fields["wrapped"] = wrapMessages
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
		Addr:     dbHost + ":5432",
	}
}
