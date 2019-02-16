package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type postTest struct {
	body string
}

var c2sTestPairs = map[string]postTest {
	"Like": {
		body: fmt.Sprintf(`{
    "type": "Like",
    "actor": "%s/actors/dc6f5f5b",
    "object": "%s/actors/dc6f5f5b/outbox/162edb32/object"
}`, apiURL, apiURL),
	},
	//"item": {
	//	body: fmt.Sprintf(""),
	//},
}

func errOnPostRequest(t *testing.T) func(postTest) {
	assertTrue := errIfNotTrue(t)
	assertGetRequest:= errOnGetRequest(t)

	return func(test postTest) {
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

		newLike, err := url.Parse(location[0])
		newLikeURL := newLike.String()
		assertTrue(err == nil, "Location header holds invalid URL %s", newLikeURL)
		assertTrue(strings.Contains(newLikeURL, apiURL), "Location header holds invalid URL %s, expected to contain %s", newLikeURL, apiURL)

		res := make(map[string]interface{})
		err = json.Unmarshal(b, &res)
		assertTrue(err == nil, "Error: unmarshal failed: %s", err)

		assertGetRequest(newLikeURL)
	}
}

func Test_C2S (t *testing.T) {
	assertPost := errOnPostRequest(t)

	for typ, test := range c2sTestPairs {
		t.Run(typ, func (t *testing.T) {
			assertPost(test)
		})
	}
}
