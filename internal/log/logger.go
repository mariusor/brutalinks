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
	l      logrus.FieldLogger
	ctx    Ctx
	oldCtx Ctx
}

func Dev() Logger {
	l := logger{}

	logrus.SetFormatter(&logrus.TextFormatter{
		QuoteEmptyFields: true,
		FullTimestamp:    false,
	})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.TraceLevel)

	l.l = logrus.StandardLogger()
	return &l
}

func Prod() Logger {
	l := logger{}

	logrus.SetFormatter(&logrus.TextFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.WarnLevel)

	l.l = logrus.StandardLogger()
	return &l
}

func (l logger) context() logrus.Fields {
	c := make(logrus.Fields, len(l.ctx))

	for k, v := range l.ctx {
		c[k] = v
	}

	return c
}

func (l logger) New(c ...interface{}) Logger {
	return l.WithContext(c)
}

func copyCtx(ctx Ctx) Ctx {
	res := make(Ctx, len(ctx))
	for k, c := range ctx {
		res[k] = c
	}
	return res
}

func (l *logger) WithContext(ctx ...interface{}) Logger {
	l.oldCtx = copyCtx(l.ctx)
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
	l.ctx = copyCtx(l.oldCtx)
	l.oldCtx = nil
}

func (l logger) Debugf(msg string, p ...interface{}) {
	l.l.WithFields(l.context()).Debug(fmt.Sprintf(msg, p...))
	l.ctx = copyCtx(l.oldCtx)
	l.oldCtx = nil
}

func (l logger) Info(msg string) {
	l.l.WithFields(l.context()).Info(msg)
	l.ctx = copyCtx(l.oldCtx)
	l.oldCtx = nil
}

func (l logger) Infof(msg string, p ...interface{}) {
	l.l.WithFields(l.context()).Info(fmt.Sprintf(msg, p...))
	l.ctx = copyCtx(l.oldCtx)
	l.oldCtx = nil
}

func (l logger) Warn(msg string) {
	l.l.WithFields(l.context()).Warn(msg)
	l.ctx = copyCtx(l.oldCtx)
	l.oldCtx = nil
}

func (l logger) Warnf(msg string, p ...interface{}) {
	l.l.WithFields(l.context()).Warn(fmt.Sprintf(msg, p...))
	l.ctx = copyCtx(l.oldCtx)
	l.oldCtx = nil
}

func (l logger) Error(msg string) {
	l.l.WithFields(l.context()).Error(msg)
	l.ctx = copyCtx(l.oldCtx)
	l.oldCtx = nil
}

func (l logger) Errorf(msg string, p ...interface{}) {
	l.l.WithFields(l.context()).Error(fmt.Sprintf(msg, p...))
	l.ctx = copyCtx(l.oldCtx)
	l.oldCtx = nil
}

func (l logger) Crit(msg string) {
	l.l.WithFields(l.context()).Fatal(msg)
	l.ctx = copyCtx(l.oldCtx)
	l.oldCtx = nil
}

func (l logger) Critf(msg string, p ...interface{}) {
	l.l.WithFields(l.context()).Fatal(fmt.Sprintf(msg, p...))
	l.ctx = copyCtx(l.oldCtx)
	l.oldCtx = nil
}

func (l logger) Print(i ...interface{}) {
	if i == nil || len(i) != 1 {
		return
	}
	l.Infof(i[0].(string))
	l.ctx = copyCtx(l.oldCtx)
	l.oldCtx = nil
}
