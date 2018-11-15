package cmd

import (
	"github.com/mariusor/littr.go/app/processing"
)

func Consume(count int) error {
	ok, nok, err := processing.ProcessMessages(count)
	Logger.Infof("messages OK:%d NOK:%d", ok, nok)
	if err != err {
		Logger.Error(err.Error())
	}
	return err
}
