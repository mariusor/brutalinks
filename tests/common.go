package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	as "github.com/go-ap/activitystreams"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"testing"
)

// UserAgent value that the client uses when performing requests
var UserAgent = "test-go-http-client"
var HeaderAccept = `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`

type postVal struct {
	body string
	res  objectVal
}

type collectionVal struct {
	id        string
	typ       string
	itemCount int64
	first     *collectionVal
	next      *collectionVal
	last      *collectionVal
	current   *collectionVal
	items     map[string]objectVal
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

var apiURL = os.Getenv("API_URL")
var host = os.Getenv("HOSTNAME")

type assertFn func(v bool, msg string, args ...interface{})
type errFn func(format string, args ...interface{})
type requestAssertFn func(iri string, met string, b io.Reader) map[string]interface{}
type requestGetAssertFn func(iri string) map[string]interface{}
type requestPostAssertFn func(iri string, b io.Reader) map[string]interface{}
type collectionAssertFn func(iri string, testVal collectionVal)
type collectionPropertiesAssertFn func(ob map[string]interface{}, testVal collectionVal)
type objectPropertiesAssertFn func(ob map[string]interface{}, testVal objectVal)
type mapFieldAssertFn func(ob map[string]interface{}, key string, testVal interface{})

func errorf(t *testing.T) errFn {
	return func(msg string, args ...interface{}) {
		msg = fmt.Sprintf("%s\n------- Stack -------\n%s\n", msg, debug.Stack())
		t.Errorf(msg, args...)
	}
}

func errIfNotTrue(t *testing.T) assertFn {
	return func(v bool, msg string, args ...interface{}) {
		if !v {
			errorf(t)(msg, args...)
		}
	}
}

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
			// the case when the mock test value is a string, but corresponds to an object in the json
			// so we need to verify the json's object id against our mock value
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
			// this is the case where the mock value is a pointer to objectVal (so we can dereference against it's id)
			// and check the subsequent properties
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
			// this is the case where the mock value is a pointer to collectionVal (so we can dereference against it's id)
			// and check the subsequent properties
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
			assertTrue(false, "UNKNOWN check for %s, %#v expected %#v", key, val, t)
		}
	}
}

func errOnObjectProperties(t *testing.T) objectPropertiesAssertFn {
	assertMapKey := errOnMapProp(t)
	assertReq := errOnGetRequest(t)
	return func(ob map[string]interface{}, tVal objectVal) {
		if tVal.id != "" {
			assertMapKey(ob, "id", tVal.id)
		}
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
			assertMapKey(ob, "inbox", tVal.inbox)
			if tVal.inbox.typ != "" {
				derefCol := assertReq(tVal.inbox.id)
				errOnCollectionProperties(t)(derefCol, *tVal.inbox)
			}
		}
		if tVal.outbox != nil {
			assertMapKey(ob, "outbox", tVal.outbox)
			if tVal.outbox.typ != "" {
				derefCol := assertReq(tVal.outbox.id)
				errOnCollectionProperties(t)(derefCol, *tVal.outbox)
			}
		}
		if tVal.liked != nil {
			assertMapKey(ob, "liked", tVal.liked)
			if tVal.liked.typ != "" {
				derefCol := assertReq(tVal.liked.id)
				errOnCollectionProperties(t)(derefCol, *tVal.liked)
			}
		}
		if tVal.act != nil {
			assertMapKey(ob, "actor", tVal.act)
			if tVal.act.typ != "" {
				derefAct := assertReq(tVal.act.id)
				errOnObjectProperties(t)(derefAct, *tVal.act)
			}
		}
		if tVal.obj != nil {
			assertMapKey(ob, "object", tVal.obj)
			if tVal.obj.id != "" {
				derefObj := assertReq(tVal.obj.id)
				errOnObjectProperties(t)(derefObj, *tVal.obj)
			}
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
		if tVal.first != nil {
			assertMapKey(ob, "first", tVal.first)
			if tVal.first.typ != "" {
				derefCol := assertReq(tVal.first.id)
				errOnCollectionProperties(t)(derefCol, *tVal.first)
			}
		}
		if tVal.next != nil {
			assertMapKey(ob, "next", tVal.next)
			if tVal.next.typ != "" {
				derefCol := assertReq(tVal.next.id)
				errOnCollectionProperties(t)(derefCol, *tVal.next)
			}
		}
		if tVal.current != nil {
			assertMapKey(ob, "current", tVal.current)
			if tVal.current.typ != "" {
				derefCol := assertReq(tVal.current.id)
				errOnCollectionProperties(t)(derefCol, *tVal.current)
			}
		}
		if tVal.last != nil {
			assertMapKey(ob, "last", tVal.last)
			if tVal.last.typ != "" {
				derefCol := assertReq(tVal.last.id)
				errOnCollectionProperties(t)(derefCol, *tVal.last)
			}
		}
		if tVal.itemCount != 0 {
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
				for k, testIt := range tVal.items {
					iri := fmt.Sprintf("%s/%s", apiURL, k)
					for _, it := range items {
						act, ok := it.(map[string]interface{})
						assertTrue(ok, "Unable to convert %#v to %T type, Received %#v:(%T)", it, act, it, it)
						itId, ok := act["id"]
						assertTrue(ok, "Could not load id property of item: %#v", act)
						itIRI, ok := itId.(string)
						assertTrue(ok, "Unable to convert %#v to %T type, Received %#v:(%T)", itId, itIRI, val, val)
						if itIRI == iri {
							t.Run(k, func(t *testing.T) {
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
}

func errOnGetRequest(t *testing.T) requestGetAssertFn {
	return func(iri string) map[string]interface{} { return errOnRequest(t)(iri, http.MethodGet, nil) }
}

func errOnRequest(t *testing.T) requestAssertFn {
	assertTrue := errIfNotTrue(t)

	return func(iri string, method string, body io.Reader) map[string]interface{} {
		b := make([]byte, 0)

		var err error
		req, err := http.NewRequest(method, iri, body)
		assertTrue(err == nil, "Error: unable to create request: %s", err)

		req.Header.Set("User-Agent", fmt.Sprintf("-%s", UserAgent))
		req.Header.Set("Accept", HeaderAccept)
		req.Header.Set("Cache-Control", "no-cache")
		resp, err := http.DefaultClient.Do(req)

		assertTrue(err == nil, "Error: request failed: %s", err)
		b, err = ioutil.ReadAll(resp.Body)
		assertTrue(resp.StatusCode == http.StatusOK,
			"Error: invalid HTTP response %d, expected %d\nResponse\n%v\n%s", resp.StatusCode, http.StatusOK, resp.Header, b)
		assertTrue(err == nil, "Error: invalid HTTP body! Read %d bytes %s", len(b), b)

		res := make(map[string]interface{})
		err = json.Unmarshal(b, &res)
		assertTrue(err == nil, "Error: unmarshal failed: %s", err)

		return res
	}
}

func errOnCollection(t *testing.T) collectionAssertFn {
	assertGetReq := errOnGetRequest(t)
	assertCollectionProperties := errOnCollectionProperties(t)

	return func(iri string, tVal collectionVal) {
		assertCollectionProperties(
			assertGetReq(iri),
			tVal,
		)
	}
}

func errOnPostRequest(t *testing.T) func(postVal) {
	assertTrue := errIfNotTrue(t)
	assertGetRequest := errOnGetRequest(t)
	assertObjectProperties := errOnObjectProperties(t)

	return func(test postVal) {
		outbox := fmt.Sprintf("%s/self/outbox", apiURL)

		body := []byte(test.body)
		b := make([]byte, 0)

		var err error
		req, err := http.NewRequest(http.MethodPost, outbox, bytes.NewReader(body))
		assertTrue(err == nil, "Error: unable to create request: %s", err)

		req.Header.Set("User-Agent", fmt.Sprintf("-%s", UserAgent))
		req.Header.Set("Accept", HeaderAccept)
		req.Header.Set("Cache-Control", "no-cache")
		resp, err := http.DefaultClient.Do(req)

		assertTrue(err == nil, "Error: request failed: %s", err)
		b, err = ioutil.ReadAll(resp.Body)
		assertTrue(resp.StatusCode == http.StatusCreated,
			"Error: invalid HTTP response %d, expected %d\nResponse\n%v\n%s", resp.StatusCode, http.StatusCreated, resp.Header, b)
		assertTrue(err == nil, "Error: invalid HTTP body! Read %d bytes %s", len(b), b)

		location, ok := resp.Header["Location"]
		assertTrue(ok, "Server didn't respond with a Location header even though it confirmed the Like was created")
		assertTrue(len(location) == 1, "Server responded with multiple Location headers which is not expected")

		newObj, err := url.Parse(location[0])
		newObjURL := newObj.String()
		assertTrue(err == nil, "Location header holds invalid URL %s", newObjURL)
		assertTrue(strings.Contains(newObjURL, apiURL), "Location header holds invalid URL %s, expected to contain %s", newObjURL, apiURL)

		if test.res.id == "" {
			test.res.id = newObjURL
		}

		res := make(map[string]interface{})
		err = json.Unmarshal(b, &res)
		assertTrue(err == nil, "Error: unmarshal failed: %s", err)

		saved := assertGetRequest(newObjURL)
		assertObjectProperties(saved, test.res)
	}
}
