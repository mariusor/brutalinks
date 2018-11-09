package cmd

import (
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app/log"
)

var Logger log.Logger

func E(errs ...error) bool {
	if len(errs) == 0 {
		return true
	}
	for _, e := range errs {
		fields := make(log.Ctx)
		var msg string
		switch err := e.(type) {
		case *errors.Err:
			if err.Underlying() != nil {
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
			}
			msg = err.Error()
		default:
			msg = err.Error()
		}
		Logger.WithContext(fields).Error(msg)
	}
	return false
}
