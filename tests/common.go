package tests

import (
	"encoding/json"
	"fmt"
	as "github.com/go-ap/activitystreams"
	"io/ioutil"
	"net/http"
	"path"
	"reflect"
	"runtime/debug"
	"testing"
)

// UserAgent value that the client uses when performing requests
var UserAgent = "test-go-http-client"
var HeaderAccept = `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`

type assertFn func(v bool, msg string, args ...interface{})
type errFn func(format string, args ...interface{})

func errorf(t *testing.T) errFn {
	return func(msg string, args ...interface{}) {
		t.Errorf(msg, args...)
		t.Fatalf("\n%s\n", debug.Stack())
	}
}

func errIfNotTrue(t *testing.T) assertFn {
	return func(v bool, msg string, args ...interface{}) {
		if !v {
			errorf(t)(msg, args...)
		}
	}
}

type collectionVal struct {
	id        string
	typ       string
	itemCount int64
	items     map[string]objectVal
}

type objectVal struct {
	id        string
	typ       string
	name      string
	url       string
	author    string
	inboxIRI  string
	outboxIRI string
	act       *objectVal
	obj       *objectVal
}
type requestAssertFn func(iri string) map[string]interface{}

type collectionAssertFn func(iri string, testVal collectionVal)
type collectionPropertiesAssertFn func(ob map[string]interface{}, testVal collectionVal)
type objectPropertiesAssertFn func(ob map[string]interface{}, testVal objectVal)
type mapFieldAssertFn func(ob map[string]interface{}, key string, testVal interface{})

func errOnMapProp(t *testing.T) mapFieldAssertFn {
	assertTrue := errIfNotTrue(t)
	return func(ob map[string]interface{}, key string, tVal interface{}) {
		val, ok := ob[key]
		assertTrue(ok, "Could not load %s property of item: %#v", key, ob)

		switch t := tVal.(type) {
		case int64, int32, int16, int8:
			v, okA := val.(float64)
			assertTrue(okA, "Unable to convert to %T type, Received %v:%T", v, val, val)
			assertTrue(int64(v) == t, "Invalid %s, %d expected %d", key, int64(v), t)
		case string, []byte:
			v := val.(string)
			assertTrue(v == t, "Invalid %s, %s expected %s", key, v, t)
		default:
			assertTrue(reflect.DeepEqual(val, t), "default Invalid %s, %#v expected %#v", key, val, t)
		}
	}
}

func errOnObjectProperties(t *testing.T) objectPropertiesAssertFn {
	assertMapKey := errOnMapProp(t)
	return func(ob map[string]interface{}, tVal objectVal) {
		assertMapKey(ob, "id", tVal.id)
		if tVal.typ != "" {
			assertMapKey(ob, "type", tVal.typ)
		}
		if tVal.name != "" {
			assertMapKey(ob, "name", tVal.name)
		}
		if tVal.url != "" {
			assertMapKey(ob, "url", tVal.url)
		}
		if tVal.inboxIRI != "" {
			assertMapKey(ob, "inbox", tVal.inboxIRI)
		}
		if tVal.author != "" {
			assertMapKey(ob, "attributedTo", tVal.author)
		}
		if tVal.act != nil {
			assertMapKey(ob, "actor", tVal.act)
		}
		if tVal.obj != nil {
			assertMapKey(ob, "object", tVal.obj)
		}
	}
}

func errOnCollectionProperties(t *testing.T) collectionPropertiesAssertFn {
	assertTrue := errIfNotTrue(t)
	assertMapKey := errOnMapProp(t)
	assertObjectProperties := errOnObjectProperties(t)

	return func(ob map[string]interface{}, tVal collectionVal) {
		assertObjectProperties(ob, objectVal{
			id:  tVal.id,
			typ: tVal.typ,
		})

		var testFirstId string
		var itemsKey string
		if tVal.typ == string(as.CollectionType) {
			testFirstId = fmt.Sprintf("%s?page=1", tVal.id)
			itemsKey = "items"
		}
		if tVal.typ == string(as.OrderedCollectionType) {
			testFirstId = fmt.Sprintf("%s?maxItems=50&page=1", tVal.id)
			itemsKey = "orderedItems"
		}
		assertMapKey(ob, "first", testFirstId)
		assertMapKey(ob, "totalItems", tVal.itemCount)

		val, ok := ob[itemsKey]
		assertTrue(ok, "Could not load %s property of item: %#v", itemsKey, ob)
		items, ok := val.([]interface{})
		assertTrue(ok, "Invalid property %s %#v, expected %T", itemsKey, val, items)
		assertTrue(len(items) == int(ob["totalItems"].(float64)),
			"Invalid item count for collection %s %d, expected %d", itemsKey, len(items), tVal.itemCount,
		)
		if len(tVal.items) > 0 {
		foundItem:
			for iri, testIt := range tVal.items {
				for _, it := range items {
					act, ok := it.(map[string]interface{})
					assertTrue(ok, "Unable to convert to %T type, Received %v:%T", act, it, it)
					itId, ok := act["id"]
					assertTrue(ok, "Could not load id property of item: %#v", act)
					if itId == iri {
						t.Run(path.Base(iri), func(t *testing.T) {
							assertObjectProperties(act, testIt)
						})
						continue foundItem
					}
				}
				errorf(t)("Unable to find %s in the %s collection %#v", iri, itemsKey, items)
			}
		}
	}
}

func errOnGetRequest(t *testing.T) requestAssertFn {
	assertTrue := errIfNotTrue(t)

	return func(iri string) map[string]interface{} {
		b := make([]byte, 0)

		var err error
		req, err := http.NewRequest(http.MethodGet, iri, nil)
		assertTrue(err == nil, "Error: unable to create request: %s", err)

		req.Header.Set("User-Agent", fmt.Sprintf("-%s", UserAgent))
		req.Header.Set("Accept", HeaderAccept)
		req.Header.Set("Cache-Control", "no-cache")
		resp, err := http.DefaultClient.Do(req)

		assertTrue(err == nil, "Error: request failed: %s", err)
		assertTrue(resp.StatusCode == http.StatusOK, "Error: invalid HTTP response %d, expected %d", resp.StatusCode, http.StatusOK)

		b, err = ioutil.ReadAll(resp.Body)
		assertTrue(err == nil, "Error: invalid HTTP body! Read %d bytes %s", len(b), b)

		res := make(map[string]interface{})
		err = json.Unmarshal(b, &res)
		assertTrue(err == nil, "Error: unmarshal failed: %s", err)

		return res
	}
}

func errOnCollection(t *testing.T) collectionAssertFn {
	assertReq := errOnGetRequest(t)
	assertCollectionProperties := errOnCollectionProperties(t)

	return func(iri string, tVal collectionVal) {
		assertCollectionProperties(
			assertReq(iri),
			tVal,
		)
	}
}
