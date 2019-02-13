package tests

import (
	"encoding/json"
	"fmt"
	as "github.com/go-ap/activitystreams"
	"golang.org/x/xerrors"
	"io"
	"io/ioutil"
	"net/http"
	"runtime/debug"
	"testing"
)

// UserAgent value that the client uses when performing requests
var UserAgent = "test-go-http-client"
var HeaderAccept = `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`

type assertFn func(v bool, msg string, args ...interface{})
type errFn func(format string, args ...interface{})

func execReq(url string, met string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(met, url, body)
	req.Header.Set("User-Agent", fmt.Sprintf("-%s", UserAgent))
	req.Header.Set("Accept", HeaderAccept)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, xerrors.Errorf("Error: invalid HTTP response %d, expected %d", resp.StatusCode, http.StatusOK)
	}
	b := make([]byte, 0)
	if b, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, xerrors.Errorf("Error: invalid HTTP body! Read %d bytes %s", len(b), b)
	}
	return b, nil
}

func errIfNotTrue(t *testing.T) assertFn {
	return func(v bool, msg string, args ...interface{}) {
		if !v {
			t.Errorf(msg, args...)
			t.Fatalf("\n%s\n", debug.Stack())
		}
	}
}

type collectionVal struct {
	iri       string
	typ       string
	itemCount int64
}
type collectionAssert func(iri string, testPair collectionVal)

func errOnCollection(t *testing.T) collectionAssert {
	assertTrue := errIfNotTrue(t)

	return func(iri string, testPair collectionVal) {
		var testFirstId string
		if testPair.typ == string(as.CollectionType) {
			testFirstId = fmt.Sprintf("%s?page=1", testPair.iri)
		}
		if testPair.typ == string(as.OrderedCollectionType) {
			testFirstId = fmt.Sprintf("%s?maxItems=50&page=1", testPair.iri)
		}

		var b []byte
		var err error
		b, err = execReq(iri, http.MethodGet, nil)
		assertTrue(err == nil, "Error %s", err)

		test := make(map[string]interface{})
		err = json.Unmarshal(b, &test)
		assertTrue(err == nil, "Error unmarshal: %s", err)

		for key, val := range test {
			if key == "id" {
				assertTrue(val == testPair.iri, "Invalid id, %s expected %s", val, testPair.iri)
			}
			if key == "type" {
				assertTrue(val == testPair.typ, "Invalid type, %s expected %s", val, testPair.typ)
			}
			if key == "totalItems" {
				v, ok := val.(float64)
				assertTrue(ok, "Unable to convert to %T type. Expected float value %v:%T", v, val, val)
				assertTrue(int64(v) == testPair.itemCount, "Invalid totalItems, %d expected %d", int64(v), testPair.itemCount)
			}
			if key == "first" {
				assertTrue(val == testFirstId, "Invalid first collection page id, %s expected %s", val, testFirstId)
			}
			if key == "items" {
				items, ok := val.([]interface{})
				assertTrue(ok, "Invalid collection items %#v, expected %T", val, items)
				assertTrue(len(items) == int(test["totalItems"].(float64)),
					"Invalid item count for collection %d, expected %d", len(items), testPair.itemCount,
				)
			}
		}
	}
}
