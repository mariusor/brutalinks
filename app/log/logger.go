package log

import (
	"fmt"
	"github.com/inconshreveable/log15"
	"os"
)

type Logger interface {
	// Add ctx value pairs
	WithContext(...interface{}) Logger
	//New(c ...interface{}) Logger

	Debug(string)
	Info(string)
	Warn(string)
	Error(string)
	Crit(string)

	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warnf(string, ...interface{})
	Errorf(string, ...interface{})
	Critf(string, ...interface{})
}

type Ctx log15.Ctx

type logger struct {
	l   log15.Logger
	ctx []interface{}
}

func Dev() Logger {
	l := logger{}
	l.l = log15.New()
	l.l.SetHandler(
		log15.MultiHandler(
			log15.LvlFilterHandler(
				log15.LvlWarn,
				log15.StreamHandler(os.Stderr, log15.LogfmtFormat()),
			),
			log15.LvlFilterHandler(
				log15.LvlDebug,
				log15.StreamHandler(os.Stdout, log15.LogfmtFormat()),
			),
		),
	)

	return &l
}

func Prod() Logger {
	l := logger{}
	l.l = log15.New()

	l.l.SetHandler(
		log15.LvlFilterHandler(
			log15.LvlWarn,
			log15.StreamHandler(os.Stderr, log15.LogfmtFormat()),
		),
	)

	return &l
}

//func (l logger) New(c ...interface{}) Logger {
//	return l.WithContext(c...)
//}

func (l *logger) WithContext(ctx ...interface{}) Logger {
	if l.ctx == nil {
		l.ctx = make([]interface{}, 0)
	}
	l.ctx = append(l.ctx, ctx...)

	return l
}

func (l logger) Debug(msg string) {
	l.l.Debug(msg, l.ctx...)
}

func (l logger) Debugf(msg string, p ...interface{}) {
	l.l.Debug(fmt.Sprintf(msg, p...), l.ctx...)
}

func (l logger) Info(msg string) {
	l.l.Info(msg, l.ctx...)
}

func (l logger) Infof(msg string, p ...interface{}) {
	l.l.Info(fmt.Sprintf(msg, p...), l.ctx...)
}

func (l logger) Warn(msg string) {
	l.l.Warn(msg, l.ctx...)
}

func (l logger) Warnf(msg string, p ...interface{}) {
	l.l.Warn(fmt.Sprintf(msg, p...), l.ctx...)
}

func (l logger) Error(msg string) {
	l.l.Error(msg, l.ctx...)
}

func (l logger) Errorf(msg string, p ...interface{}) {
	l.l.Error(fmt.Sprintf(msg, p...), l.ctx...)
}

func (l logger) Crit(msg string) {
	l.l.Crit(msg, l.ctx...)
}

func (l logger) Critf(msg string, p ...interface{}) {
	l.l.Crit(fmt.Sprintf(msg, p...), l.ctx...)
}
