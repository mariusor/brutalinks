package tests

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"fmt"
	as "github.com/go-ap/activitystreams"
	"github.com/go-pg/pg"
	_ "github.com/joho/godotenv/autoload"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/cmd"
	"github.com/spacemonkeygo/httpsig"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// UserAgent value that the client uses when performing requests
var UserAgent = "test-go-http-client"
var HeaderAccept = `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`

type testPairs map[string][]testPair

type testAccount struct {
	id         string
	publicKey  crypto.PublicKey
	privateKey crypto.PrivateKey
}

type testReq struct {
	met     string
	url     string
	headers http.Header
	account *testAccount
	body    string
}

type testRes struct {
	code int
	val  objectVal
	body string
}

type testPair struct {
	req testReq
	res testRes
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
	inbox             *objectVal
	outbox            *objectVal
	following         *objectVal
	liked             *objectVal
	act               *objectVal
	obj               *objectVal
	itemCount         int64
	first             *objectVal
	next              *objectVal
	last              *objectVal
	current           *objectVal
	items             map[string]objectVal
}

var (
	apiURL = os.Getenv("API_URL")
	host   = os.Getenv("HOSTNAME")
	o      *pg.Options
	r      *pg.Options
)

func init() {
	dbRootPw := os.Getenv("POSTGRES_PASSWORD")
	dbRootUser := "postgres"
	dbRootName := "postgres"
	o = cmd.PGConfigFromENV()
	r = &pg.Options{
		User:     dbRootUser,
		Password: dbRootPw,
		Database: dbRootName,
		Addr:     o.Addr,
	}
	if errs := cmd.DestroyDB(r, o.User, o.Database); len(errs) > 0 {
		fmt.Printf("%v\n", errs)
	}
	if err := cmd.CreateDatabase(o, r); err != nil {
		panic(err)
	}
}

const testActorHash = "f00f00f00f00f00f00f00f00f00f6667"

var outboxURL = fmt.Sprintf("%s/self/outbox", apiURL)
var rnd = rand.New(rand.NewSource(6667))
var key, _ = rsa.GenerateKey(rnd, 512)
var prv, _ = x509.MarshalPKCS8PrivateKey(key)
var pub, _ = x509.MarshalPKIXPublicKey(&key.PublicKey)
var meta = app.AccountMetadata{
	ID: fmt.Sprintf("%s/self/following/%s", apiURL, testActorHash),
	Key: &app.SSHKey{
		ID:      fmt.Sprintf("%s/self/following/%s#main-key", apiURL, testActorHash),
		Public:  pub,
		Private: prv,
	},
}
var defaultTestAccount = testAccount{
	id:         meta.ID,
	publicKey:  key.Public(),
	privateKey: key,
}
var jm, _ = json.Marshal(meta)
var data = map[string][][]interface{}{
	"accounts": {
		{
			interface{}(666),
			interface{}(testActorHash),
			interface{}("johndoe"),
			interface{}(fmt.Sprintf("jd@%s", host)),
			interface{}(string(jm)),
		},
	},
}

func resetDB(t *testing.T, testData bool) {
	h := os.Getenv("HOSTNAME")

	t.Helper()
	//t.Logf("Resetting DB")
	if err := cmd.BootstrapDB(o); err != nil {
		t.Log(err)
	}
	if err := cmd.SeedDB(o, h); err != nil {
		t.Fatal(err)
	}
	if testData {
		if err := cmd.SeedTestData(o, data); err != nil {
			t.Fatal(err)
		}
	}
}

type assertFn func(v bool, msg string, args ...interface{})
type errFn func(format string, args ...interface{})
type requestGetAssertFn func(iri string) map[string]interface{}
type objectPropertiesAssertFn func(ob map[string]interface{}, testVal objectVal)
type mapFieldAssertFn func(ob map[string]interface{}, key string, testVal interface{})

func errorf(t *testing.T) errFn {
	return func(msg string, args ...interface{}) {
		msg = fmt.Sprintf("%s\n------- Stack -------\n%s\n", msg, debug.Stack())
		if args == nil || len(args) == 0 {
			return
		}
		t.Errorf(msg, args...)
		t.FailNow()
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
		t.Run(key, func(t *testing.T) {
			val, ok := ob[key]
			errIfNotTrue(t)(ok, "Could not load %s property of item: %#v", key, ob)

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
			default:
				assertTrue(false, "UNKNOWN check for %s, %#v expected %#v", key, val, t)
			}
		})
	}
}

func errOnObjectProperties(t *testing.T) objectPropertiesAssertFn {
	assertMapKey := errOnMapProp(t)
	assertReq := errOnGetRequest(t)
	assertTrue := errIfNotTrue(t)

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
				dCol := assertReq(tVal.inbox.id)
				errOnObjectProperties(t)(dCol, *tVal.inbox)
			}
		}
		if tVal.outbox != nil {
			assertMapKey(ob, "outbox", tVal.outbox)
			if tVal.outbox.typ != "" {
				dCol := assertReq(tVal.outbox.id)
				errOnObjectProperties(t)(dCol, *tVal.outbox)
			}
		}
		if tVal.liked != nil {
			assertMapKey(ob, "liked", tVal.liked)
			if tVal.liked.typ != "" {
				dCol := assertReq(tVal.liked.id)
				errOnObjectProperties(t)(dCol, *tVal.liked)
			}
		}
		if tVal.following != nil {
			assertMapKey(ob, "following", tVal.following)
			if tVal.following.typ != "" {
				dCol := assertReq(tVal.following.id)
				errOnObjectProperties(t)(dCol, *tVal.following)
			}
		}
		if tVal.act != nil {
			assertMapKey(ob, "actor", tVal.act)
			if tVal.act.typ != "" {
				dAct := assertReq(tVal.act.id)
				errOnObjectProperties(t)(dAct, *tVal.act)
			}
		}
		if tVal.obj != nil {
			assertMapKey(ob, "object", tVal.obj)
			if tVal.obj.id != "" {
				derefObj := assertReq(tVal.obj.id)
				errOnObjectProperties(t)(derefObj, *tVal.obj)
			}
		}
		if tVal.typ != string(as.CollectionType) && tVal.typ != string(as.OrderedCollectionType) && tVal.typ != string(as.CollectionPageType) && tVal.typ != string(as.OrderedCollectionPageType) {
			return
		}
		if tVal.first != nil {
			assertMapKey(ob, "first", tVal.first)
			if tVal.first.typ != "" {
				derefCol := assertReq(tVal.first.id)
				errOnObjectProperties(t)(derefCol, *tVal.first)
			}
		}
		if tVal.next != nil {
			assertMapKey(ob, "next", tVal.next)
			if tVal.next.typ != "" {
				derefCol := assertReq(tVal.next.id)
				errOnObjectProperties(t)(derefCol, *tVal.next)
			}
		}
		if tVal.current != nil {
			assertMapKey(ob, "current", tVal.current)
			if tVal.current.typ != "" {
				dCol := assertReq(tVal.current.id)
				errOnObjectProperties(t)(dCol, *tVal.current)
			}
		}
		if tVal.last != nil {
			assertMapKey(ob, "last", tVal.last)
			if tVal.last.typ != "" {
				derefCol := assertReq(tVal.last.id)
				errOnObjectProperties(t)(derefCol, *tVal.last)
			}
		}
		if tVal.itemCount != 0 {
			assertMapKey(ob, "totalItems", tVal.itemCount)
			itemsKey := func(typ string) string {
				if typ == string(as.CollectionType) {
					return "items"
				}
				return "orderedItems"
			}(tVal.typ)
			if len(tVal.items) > 0 {
				val, ok := ob[itemsKey]
				assertTrue(ok, "Could not load %s property of collection: %#v\n\n%#v\n\n", itemsKey, ob, tVal.items)
				items, ok := val.([]interface{})
				assertTrue(ok, "Invalid property %s %#v, expected %T", itemsKey, val, items)
				assertTrue(len(items) == int(ob["totalItems"].(float64)),
					"Invalid item count for collection %s %d, expected %d", itemsKey, len(items), tVal.itemCount,
				)
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
						if strings.EqualFold(itIRI, iri) {
							t.Run(k, func(t *testing.T) {
								errOnObjectProperties(t)(act, testIt)
								dAct := assertReq(itIRI)
								errOnObjectProperties(t)(dAct, testIt)
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
func errOnGetRequest(t *testing.T) func(string) map[string]interface{} {
	return func(iri string) map[string]interface{} {
		tVal := testPair{
			req: testReq{
				met: http.MethodGet,
				url: iri,
			},
			res: testRes{
				code: http.StatusOK,
			},
		}
		return errOnRequest(t)(tVal)
	}
}

var signHdrs = []string{"(request-target)", "host", "date"}

func errOnRequest(t *testing.T) func(testPair) map[string]interface{} {
	assertTrue := errIfNotTrue(t)
	assertGetRequest := errOnGetRequest(t)
	assertObjectProperties := errOnObjectProperties(t)

	return func(test testPair) map[string]interface{} {
		if len(test.req.headers) == 0 {
			test.req.headers = make(http.Header, 0)
			test.req.headers.Set("User-Agent", fmt.Sprintf("-%s", UserAgent))
			test.req.headers.Set("Accept", HeaderAccept)
			test.req.headers.Set("Cache-Control", "no-cache")
		}
		if test.req.met == "" {
			test.req.met = http.MethodPost
		}
		if test.res.code == 0 {
			test.res.code = http.StatusCreated
		}

		body := []byte(test.req.body)
		b := make([]byte, 0)

		var err error
		req, err := http.NewRequest(test.req.met, test.req.url, bytes.NewReader(body))
		assertTrue(err == nil, "Error: unable to create request: %s", err)

		req.Header = test.req.headers
		if test.req.account != nil {
			req.Header.Add("Date", time.Now().Format(http.TimeFormat))
			keyId := fmt.Sprintf("%s#main-key", test.req.account.id)
			s := httpsig.NewSigner(keyId, test.req.account.privateKey, httpsig.RSASHA256, signHdrs)
			err := s.Sign(req)
			assertTrue(err == nil, "Error: unable to sign request: %s", err)
		}
		resp, err := http.DefaultClient.Do(req)

		assertTrue(err == nil, "Error: request failed: %s", err)
		assertTrue(resp.StatusCode == test.res.code,
			"Error: invalid HTTP response %d, expected %d\nResponse\n%v\n%s", resp.StatusCode, test.res.code, resp.Header, b)

		b, err = ioutil.ReadAll(resp.Body)
		assertTrue(err == nil, "Error: invalid HTTP body! Read %d bytes %s", len(b), b)
		if test.req.met != http.MethodGet {
			location, ok := resp.Header["Location"]
			if !ok {
				return nil
			}
			assertTrue(ok, "Server didn't respond with a Location header even though it confirmed the Like was created")
			assertTrue(len(location) == 1, "Server responded with %d Location headers which is not expected", len(location))

			newObj, err := url.Parse(location[0])
			newObjURL := newObj.String()
			assertTrue(err == nil, "Location header holds invalid URL %s", newObjURL)
			assertTrue(strings.Contains(newObjURL, apiURL), "Location header holds invalid URL %s, expected to contain %s", newObjURL, apiURL)

			if test.res.val.id == "" {
				test.res.val.id = newObjURL
			}
		}

		res := make(map[string]interface{})
		err = json.Unmarshal(b, &res)
		assertTrue(err == nil, "Error: unmarshal failed: %s", err)

		if test.res.val.id != "" {
			saved := assertGetRequest(test.res.val.id)
			if test.res.val.typ != "" {
				assertObjectProperties(saved, test.res.val)
			}
		}

		return res
	}
}
