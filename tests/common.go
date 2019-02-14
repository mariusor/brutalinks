package tests

import (
	"encoding/json"
	"fmt"
	as "github.com/go-ap/activitystreams"
	"io/ioutil"
	"net/http"
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
	id         string
	typ        string
	itemCount  int64
	first      *collectionVal
	next       *collectionVal
	last       *collectionVal
	current    *collectionVal
	items      map[string]objectVal
}

type objectVal struct {
	id                string
	typ               string
	name              string
	preferredUsername string
	summary           string
	url               string
	score             int64
	content           string
	mediaType         string
	author            string
	inbox             *collectionVal
	outbox            *collectionVal
	liked             *collectionVal
	act               *objectVal
	obj               *objectVal
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

		switch tt := tVal.(type) {
		case int64, int32, int16, int8:
			v, okA := val.(float64)
			assertTrue(okA, "Unable to convert %#v to %T type, Received %#v:(%T)", val, v, val, val)
			assertTrue(int64(v) == tt, "Invalid %s, %d expected %d", key, int64(v), tt)
		case string, []byte:
			v1, okA := val.(string)
			v2, okB := val.(map[string]interface{})
			assertTrue(okA || okB, "Unable to convert %#v to %T or %T types, Received %#v:(%T)", val, v1, v2, val, val)
			if okA {
				assertTrue(v1 == tt, "Invalid %s, %s expected %s", key, v1, tt)
			}
			if okB {
				errOnMapProp(t)(v2, "id", tt)
			}
		case *objectVal:
			if tt != nil {
				v1, okA := val.(string)
				v2, okB := val.(map[string]interface{})
				assertTrue(okA || okB, "Unable to convert %#v to %T or %T types, Received %#v:(%T)", val, v1, v2, val, val)
				if okA {
					assertTrue(v1 == tt.id, "Invalid %s, %s expected in %#v", "id", v1, tt)
				}
				if okB {
					errOnObjectProperties(t)(v2, *tt)
				}
			}
		case *collectionVal:
			if tt != nil {
				assertTrue(tt != nil, "NIL pointer received as test val %#v(%T)", tt, tt)
				v1, okA := val.(string)
				v2, okB := val.(map[string]interface{})
				assertTrue(okA || okB, "Unable to convert %#v to %T or %T types, Received %#v:(%T)", val, v1, v2, val, val)
				if okA {
					assertTrue(v1 == tt.id, "Invalid %s, %s expected in %#v", "id", v1, tt)
				}
				if okB {
					errOnCollectionProperties(t)(v2, *tt)
				}
			}
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
		if tVal.preferredUsername != "" {
			assertMapKey(ob, "preferredUsername", tVal.preferredUsername)
		}
		if tVal.score != 0 {
			assertMapKey(ob, "score", tVal.score)
		}
		if tVal.url != "" {
			assertMapKey(ob, "url", tVal.url)
		}
		if tVal.author != "" {
			assertMapKey(ob, "attributedTo", tVal.author)
		}
		if tVal.inbox != nil {
			assertMapKey(ob, "inbox", tVal.inbox.id)
		}
		if tVal.outbox != nil {
			assertMapKey(ob, "outbox", tVal.outbox.id)
		}
		if tVal.liked != nil {
			assertMapKey(ob, "liked", tVal.liked.id)
		}
		if tVal.act != nil {
			assertMapKey(ob, "actor", tVal.act.id)
		}
		if tVal.obj != nil {
			assertMapKey(ob, "object", tVal.obj.id)
		}
	}
}

func errOnCollectionProperties(t *testing.T) collectionPropertiesAssertFn {
	assertTrue := errIfNotTrue(t)
	assertReq := errOnGetRequest(t)
	assertMapKey := errOnMapProp(t)
	assertObjectProperties := errOnObjectProperties(t)

	return func(ob map[string]interface{}, tVal collectionVal) {
		assertObjectProperties(ob, objectVal{
			id:  tVal.id,
			typ: tVal.typ,
		})

		itemsKey := func(typ string) string {
			if typ == string(as.CollectionType) {
				return "items"
			}
			return "orderedItems"
		}(tVal.typ)
		assertMapKey(ob, "first", tVal.first)
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
					assertTrue(ok, "Unable to convert %#v to %T type, Received %#v:(%T)", it, act, it, it)
					itId, ok := act["id"]
					assertTrue(ok, "Could not load id property of item: %#v", act)
					itIRI, ok := itId.(string)
					assertTrue(ok, "Unable to convert %#v to %T type, Received %#v:(%T)", itId, itIRI, val, val)
					if itIRI == iri {
						t.Run(iri, func(t *testing.T) {
							assertObjectProperties(act, testIt)
							derefAct := assertReq(itIRI)
							assertObjectProperties(derefAct, testIt)
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
