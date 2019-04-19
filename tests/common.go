package tests

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	as "github.com/go-ap/activitystreams"
	"github.com/go-chi/chi"
	"github.com/go-pg/pg"
	_ "github.com/joho/godotenv/autoload"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/api"
	"github.com/mariusor/littr.go/app/cmd"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/oauth"
	"github.com/mariusor/littr.go/internal/errors"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/openshift/osin"
	"github.com/spacemonkeygo/httpsig"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
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
	Hash       string `json:"hash"`
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
	partOf            *objectVal
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

const testActorHash = "f00f00f00f00f00f00f00f00f00f6667"

var outboxURL = fmt.Sprintf("%s/self/outbox", apiURL)
var baseURL = strings.Replace(apiURL, "/api", "", 1)
var callbackURL = fmt.Sprintf("%s/auth/local/callback", baseURL)
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
	id:         fmt.Sprintf("%s/self/following/%s", apiURL, testActorHash),
	Hash:       testActorHash,
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

func runAPP() {
	e := app.EnvType(app.TEST)
	app.Instance = app.New(host, 3001, e, "-git")

	db.Init(&app.Instance)
	defer db.Config.DB.Close()

	oauth2, err := oauth.NewOAuth(
		app.Instance.Config.DB.Host,
		app.Instance.Config.DB.User,
		app.Instance.Config.DB.Pw,
		app.Instance.Config.DB.Name,
		app.Instance.Logger.New(log.Ctx{"package": "oauth2"}),
	)
	if err != nil {
		panic(err.Error())
	}
	a := api.Init(api.Config{
		Logger:  app.Instance.Logger.New(log.Ctx{"package": "api"}),
		BaseURL: app.Instance.APIURL,
		OAuth2:  oauth2,
	})

	db.Logger = app.Instance.Logger.New(log.Ctx{"package": "db"})

	r := chi.NewRouter()
	r.With(db.Repository).Route("/api", a.Routes())
	app.Instance.Run(r, 1)
}

func createDB() {
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
	if err := cmd.CreateDatabase(o, r); err != nil {
		panic(err)
	}
	if err := cmd.BootstrapDB(o); err != nil {
		panic(err)
	}
}

func resetDB(t *testing.T, testData bool) {
	t.Helper()
	t.Logf("Resetting DB")
	if err := cmd.CleanDB(o); err != nil {
		t.Log(err)
	}
	if err := cmd.SeedDB(o, host); err != nil {
		t.Fatal(err)
	}
	if testData {
		if err := cmd.SeedTestData(o, data); err != nil {
			t.Fatal(err)
		} else {
			t.Logf("Seeded database with test data")
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
		if tVal.typ != string(as.CollectionType) &&
			tVal.typ != string(as.OrderedCollectionType) &&
			tVal.typ != string(as.CollectionPageType) &&
			tVal.typ != string(as.OrderedCollectionPageType) {
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
		if tVal.partOf != nil {
			assertMapKey(ob, "partOf", tVal.partOf)
			if tVal.partOf.typ != "" {
				derefCol := assertReq(tVal.partOf.id)
				errOnObjectProperties(t)(derefCol, *tVal.partOf)
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
							kk := strings.Replace(k, "self/", "", 1)
							t.Run(kk, func(t *testing.T) {
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
func errOnGetRequest(t *testing.T) requestGetAssertFn {
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

func osinServer() (*osin.Server, error) {
	parts := strings.Split(o.Addr, ":")
	pg := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable", parts[0], o.User, o.Password, o.Database)
	db, err := sql.Open("postgres", pg)
	if err != nil {
		return nil, err
	}

	config := osin.ServerConfig{
		AuthorizationExpiration:   86400,
		AccessExpiration:          2678400,
		TokenType:                 "Bearer",
		AllowedAuthorizeTypes:     osin.AllowedAuthorizeType{osin.CODE},
		AllowedAccessTypes:        osin.AllowedAccessType{osin.AUTHORIZATION_CODE},
		ErrorStatusCode:           http.StatusForbidden,
		AllowClientSecretInParams: true,
		AllowGetAccessRequest:     false,
		RetainTokenAfterRefresh:   false,
	}
	return osin.NewServer(&config, oauth.New(db, log.Dev())), nil
}

func osinAccess(s *osin.Server) (*osin.Response, error) {
	resp := s.NewResponse()
	defer resp.Close()

	v := url.Values{}
	v.Add("request_uri", url.QueryEscape("http://127.0.0.3/auth/local/callback"))
	v.Add("client_id", os.Getenv("OAUTH2_KEY"))
	v.Add("response_type", "code")
	dummyAuthReq, _ := http.NewRequest(http.MethodGet, "/oauth2/authorize", nil)
	dummyAuthReq.URL.RawQuery = v.Encode()
	ar := s.HandleAuthorizeRequest(resp, dummyAuthReq)
	if ar == nil {
		return resp, errors.BadRequestf("invalid authorize req")
	}
	b, _ := json.Marshal(defaultTestAccount)
	ar.UserData = b
	ar.Authorized = true
	s.FinishAuthorizeRequest(resp, dummyAuthReq, ar)

	return resp, nil
}

func addOAuth2Auth(r *http.Request, a *testAccount, s *osin.Server) error {
	resp, err := osinAccess(s)
	if err != nil {
		return err
	}
	if d := resp.Output["code"]; d != nil {
		cod, ok := d.(string)
		if !ok {
			return errors.BadRequestf("unable to finish authorize req, bad response code %s", cod)
		}
		resp := s.NewResponse()
		defer resp.Close()
		key := os.Getenv("OAUTH2_KEY")
		sec := os.Getenv("OAUTH2_SECRET")
		v := url.Values{}
		v.Add("request_uri", url.QueryEscape("http://127.0.0.3/auth/local/callback"))
		v.Add("client_id", key)
		v.Add("client_secret", sec)
		v.Add("access_type", "online")
		v.Add("grant_type", "authorization_code")
		v.Add("state", "state")
		v.Add("code", cod)
		dummyAccessReq, _ := http.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(v.Encode()))
		dummyAccessReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ar := s.HandleAccessRequest(resp, dummyAccessReq)
		if ar == nil {
			return errors.BadRequestf("invalid access req")
		}
		ar.Authorized = true
		b, _ := json.Marshal(a)
		ar.UserData = b
		ar.ForceAccessData = ar.AccessData
		s.FinishAccessRequest(resp, dummyAccessReq, ar)

		if cod := resp.Output["access_token"]; d != nil {
			tok, okK := cod.(string)
			typ, okP := resp.Output["token_type"].(string)
			if okK && okP {
				r.Header.Set("Authorization", fmt.Sprintf("%s %s", typ, tok))
				return nil
			}
		}
		return errors.BadRequestf("unable to finish access req, bad response token %s", cod)
	}
	return errors.New("unknown :D")
}

func errOnRequest(t *testing.T) func(testPair) map[string]interface{} {
	assertTrue := errIfNotTrue(t)
	assertGetRequest := errOnGetRequest(t)
	assertObjectProperties := errOnObjectProperties(t)

	oauthServ, err := osinServer()
	if err != nil {
		t.Errorf("%s", err)
	}

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
		if test.req.account != nil {
			t.Skipf("Let's skip authenticated requests for now")
		}

		body := []byte(test.req.body)
		b := make([]byte, 0)

		var err error
		req, err := http.NewRequest(test.req.met, test.req.url, bytes.NewReader(body))
		assertTrue(err == nil, "Error: unable to create request: %s", err)

		req.Header = test.req.headers
		if test.req.account != nil {
			req.Header.Set("Date", time.Now().Format(http.TimeFormat))
			var err error
			if path.Base(req.URL.Path) == "inbox" {
				err = httpsig.NewSigner(
					fmt.Sprintf("%s#main-key", test.req.account.id),
					test.req.account.privateKey,
					httpsig.RSASHA256,
					signHdrs,
				).Sign(req)
			}
			if path.Base(req.URL.Path) == "outbox" {
				err = addOAuth2Auth(req, test.req.account, oauthServ)
			}
			assertTrue(err == nil, "Error: unable to sign request: %s", err)
		}
		resp, err := http.DefaultClient.Do(req)

		assertTrue(err == nil, "Error: request failed: %s", err)
		assertTrue(resp.StatusCode == test.res.code,
			"Error: invalid HTTP response %d, expected %d\nReq:[%s] %s\n%v\nResponse\n%v\n%s",
			resp.StatusCode, test.res.code, req.Method, req.URL, req.Header, resp.Header, b)

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
