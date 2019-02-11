package tests

import (
	"encoding/json"
	"fmt"
	"golang.org/x/xerrors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	as "github.com/go-ap/activitystreams"
)

// UserAgent value that the client uses when performing requests
var UserAgent = "test-go-http-client"
var HeaderAccept = `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`

func TestEmpty(t *testing.T) {
	t.Logf("OK")
}

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

func Test_GETNodeInfo(t *testing.T) {
	apiURL := os.Getenv("API_URL")

	host := os.Getenv("HOSTNAME")

	testId := fmt.Sprintf("http://%s/api/self", host)
	testUrl := fmt.Sprintf("http://%s", host)
	testOutbox := fmt.Sprintf("%s/api/self/outbox", testUrl)
	testInbox := fmt.Sprintf("%s/api/self/inbox", testUrl)
	testAuthor := "https://github.com/mariusor"

	url := fmt.Sprintf("%s/self", apiURL)
	var b []byte
	var err error
	if b, err = execReq(url, http.MethodGet, nil); err != nil {
		t.Errorf("Error %s", err)
	}
	test := make(map[string]interface{})
	if err = json.Unmarshal(b, &test); err != nil {
		t.Errorf("Error unmarshal: %s", err)
	}
	for key, val := range test {
		if key == "id" {
			if val != testId {
				t.Errorf("Invalid id, %s expected %s", val, testId)
			}
		}
		if key == "type" {
			if val != string(as.ServiceType) {
				t.Errorf("Invalid type, %s expected %s", val, as.ServiceType)
			}
		}
		if key == "url" {
			if val != testUrl {
				t.Errorf("Invalid url, %s expected %s", val, testUrl)
			}
		}
		if key == "outbox" {
			if val != testOutbox {
				t.Errorf("Invalid outbox url, %s expected %s", val, testOutbox)
			}
		}
		if key == "inbox" {
			if val != testInbox {
				t.Errorf("Invalid inbox url, %s expected %s", val, testInbox)
			}
		}
		if key == "attributedTo" {
			if val != testAuthor {
				t.Errorf("Invalid author, %s expected %s", val, testAuthor)
			}
		}
	}
	t.Logf("%#v", test)
}
