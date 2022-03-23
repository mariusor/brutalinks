package log

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/sirupsen/logrus"
)

type Level int8

const (
	PanicLevel Level = iota
	FatalLevel
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
	TraceLevel
)

type Logger interface {
	// Add ctx value pairs
	WithContext(...Ctx) Logger
	New(c ...Ctx) Logger

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

	middleware.LogFormatter
}

type Ctx map[string]interface{}

type logger struct {
	l       logrus.FieldLogger
	ctx     Ctx
	prevCtx Ctx
	m       sync.RWMutex
}

var logFormatter = logrus.TextFormatter{
	ForceColors:            true,
	TimestampFormat:        time.StampMilli,
	FullTimestamp:          true,
	DisableSorting:         true,
	DisableLevelTruncation: false,
	PadLevelText:           true,
	QuoteEmptyFields:       false,
}

func Dev(lvl Level) Logger {
	l := new(logger)

	logger := logrus.New()
	logger.Formatter = &logFormatter
	logger.Level = logrus.Level(lvl)
	logger.Out = os.Stdout
	l.l = logger
	l.ctx = Ctx{}
	return l
}

func Prod() Logger {
	l := logger{}

	logrus.SetFormatter(&logrus.TextFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.Level(WarnLevel))

	l.l = logrus.StandardLogger()
	l.ctx = Ctx{}
	return &l
}

func context(l *logger) logrus.Fields {
	l.m.RLock()
	defer l.m.RUnlock()

	c := make(logrus.Fields, len(l.ctx))
	for k, v := range l.ctx {
		c[k] = v
	}

	return c
}

func (l logger) New(c ...Ctx) Logger {
	return l.WithContext(c...)
}

func (l *logger) WithContext(ctx ...Ctx) Logger {
	l.m.Lock()
	defer l.m.Unlock()
	l.prevCtx = l.ctx
	l.ctx = Ctx{}
	for _, c := range ctx {
		for k, v := range c {
			l.ctx[k] = v
		}
	}
	return l
}

func (l *logger) Debug(msg string) {
	l.l.WithFields(context(l)).Debug(msg)
	l.ctx = l.prevCtx
	l.prevCtx = Ctx{}
}

func (l *logger) Debugf(msg string, p ...interface{}) {
	l.l.WithFields(context(l)).Debug(fmt.Sprintf(msg, p...))
	l.ctx = l.prevCtx
	l.prevCtx = Ctx{}
}

func (l *logger) Info(msg string) {
	l.l.WithFields(context(l)).Info(msg)
	l.ctx = l.prevCtx
	l.prevCtx = Ctx{}
}

func (l *logger) Infof(msg string, p ...interface{}) {
	l.l.WithFields(context(l)).Info(fmt.Sprintf(msg, p...))
	l.ctx = l.prevCtx
	l.prevCtx = Ctx{}
}

func (l *logger) Warn(msg string) {
	l.l.WithFields(context(l)).Warn(msg)
	l.ctx = l.prevCtx
	l.prevCtx = Ctx{}
}

func (l *logger) Warnf(msg string, p ...interface{}) {
	l.l.WithFields(context(l)).Warn(fmt.Sprintf(msg, p...))
	l.ctx = l.prevCtx
	l.prevCtx = Ctx{}
}

func (l *logger) Error(msg string) {
	l.l.WithFields(context(l)).Error(msg)
	l.ctx = l.prevCtx
	l.prevCtx = Ctx{}
}

func (l *logger) Errorf(msg string, p ...interface{}) {
	l.l.WithFields(context(l)).Error(fmt.Sprintf(msg, p...))
	l.ctx = l.prevCtx
	l.prevCtx = Ctx{}
}

func (l *logger) Crit(msg string) {
	l.l.WithFields(context(l)).Fatal(msg)
	l.ctx = Ctx{}
}

func (l *logger) Critf(msg string, p ...interface{}) {
	l.l.WithFields(context(l)).Fatal(fmt.Sprintf(msg, p...))
	l.ctx = l.prevCtx
	l.prevCtx = Ctx{}
}

func (l *logger) Print(i ...interface{}) {
	if i == nil || len(i) != 1 {
		return
	}
	l.Infof(i[0].(string))
}

type log struct {
	c Ctx
	m sync.RWMutex
	l *logger
}

func (l *log) Write(status, bytes int, h http.Header, elapsed time.Duration, _ interface{}) {
	l.m.Lock()
	defer l.m.Unlock()

	l.c["duration"] = elapsed
	l.c["length"] = bytes
	l.c["status"] = status

	st := "OK"
	fn := l.l.WithContext(l.c).Info
	if status >= 400 {
		st = "FAIL"
		fn = l.l.WithContext(l.c).Warn
	}
	fn(st)
}

func (l *log) Panic(v interface{}, stack []byte) {
	l.c["stack"] = stack
	l.c["v"] = v

	l.l.WithContext(l.c).Crit("")
}

func (l *logger) NewLogEntry(r *http.Request) middleware.LogEntry {
	ll := log{c: Ctx{}, l: l}
	ll.c["met"] = r.Method
	ll.c["host"] = r.Host
	ll.c["uri"] = r.RequestURI
	ll.c["proto"] = r.Proto
	ll.c["https"] = false
	if reqID := middleware.GetReqID(r.Context()); reqID != "" {
		ll.c["id"] = reqID
	}
	if r.TLS != nil {
		ll.c["https"] = true
	}
	return &ll
}
