package log

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
)

type Logger interface {
	// Add ctx value pairs
	WithContext(...interface{}) Logger
	New(c ...interface{}) Logger

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

	Print(...interface{})
}

type Ctx logrus.Fields

type logger struct {
	l   logrus.FieldLogger
	ctx Ctx
}

func Dev() Logger {
	l := logger{}
	l.l = logrus.New()

	logrus.SetFormatter(&logrus.TextFormatter{
		QuoteEmptyFields:true,
		FullTimestamp: false,
	})
	logrus.SetReportCaller(true)
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.TraceLevel)
	return &l
}

func Prod() Logger {
	l := logger{}
	l.l = logrus.New()

	logrus.SetFormatter(&logrus.TextFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.WarnLevel)

	return &l
}

func (l logger) context()  logrus.Fields {
	c := make(logrus.Fields, len(l.ctx))

	for k, v := range l.ctx {
		c[k] = v
	}

	return c
}

func (l logger) New(c ...interface{}) Logger {
	return l.WithContext(c)
}

func (l *logger) WithContext(ctx ...interface{}) Logger {
	if l.ctx == nil {
		l.ctx = make(Ctx, 0)
	}
	for _, c := range ctx {
		switch cc := c.(type) {
		case Ctx:
			for k, v := range cc {
				l.ctx[k] = v
			}
		}
	}

	return l
}

func (l logger) Debug(msg string) {
	l.l.WithFields(l.context()).Debug(msg)
}

func (l logger) Debugf(msg string, p ...interface{}) {
	l.l.WithFields(l.context()).Debug(fmt.Sprintf(msg, p...))
}

func (l logger) Info(msg string) {
	l.l.WithFields(l.context()).Info(msg)
}

func (l logger) Infof(msg string, p ...interface{}) {
	l.l.WithFields(l.context()).Info(fmt.Sprintf(msg, p...))
}

func (l logger) Warn(msg string) {
	l.l.WithFields(l.context()).Warn(msg)
}

func (l logger) Warnf(msg string, p ...interface{}) {
	l.l.WithFields(l.context()).Warn(fmt.Sprintf(msg, p...))
}

func (l logger) Error(msg string) {
	l.l.WithFields(l.context()).Error(msg)
}

func (l logger) Errorf(msg string, p ...interface{}) {
	l.l.WithFields(l.context()).Error(fmt.Sprintf(msg, p...))
}

func (l logger) Crit(msg string) {
	l.l.WithFields(l.context()).Fatal(msg)
}

func (l logger) Critf(msg string, p ...interface{}) {
	l.l.WithFields(l.context()).Fatal(fmt.Sprintf(msg, p...))
}

func (l logger) Print(i ...interface{}) {
	if i == nil || len(i) != 1 {
		return
	}
	l.Infof(i[0].(string))
}
