package tests

import "testing"

// UserAgent value that the client uses when performing requests
var UserAgent = "test-go-http-client"
var HeaderAccept = `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`

type assertFn func(v bool, msg string, args ...interface{})
type errFn func(format string, args ...interface{})

func errIfNot(t *testing.T) assertFn {
	return func(v bool, msg string, args ...interface{}) {
		if !v {
			t.Errorf(msg, args...)
			panic(nil)
		}
	}
}
