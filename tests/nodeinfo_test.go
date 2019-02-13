package tests

import (
	"encoding/json"
	"fmt"
	as "github.com/go-ap/activitystreams"
	"net/http"
	"os"
	"testing"
)

func Test_GETNodeInfo(t *testing.T) {
	apiURL := os.Getenv("API_URL")
	host := os.Getenv("HOSTNAME")

	testId := fmt.Sprintf("%s/self", apiURL)
	testUrl := fmt.Sprintf("http://%s", host)
	testOutbox := fmt.Sprintf("%s/outbox", testId)
	testInbox := fmt.Sprintf("%s/inbox", testId)
	testAuthor := "https://github.com/mariusor"

	assertTrue := errIfNotTrue(t)

	url := fmt.Sprintf("%s/self", apiURL)
	var b []byte
	var err error

	b, err = execReq(url, http.MethodGet, nil)
	assertTrue(err == nil, "Error %s", err)

	test := make(map[string]interface{})

	err = json.Unmarshal(b, &test)
	assertTrue(err == nil, "Error unmarshal: %s", err)

	for key, val := range test {
		if key == "id" {
			assertTrue(val == testId, "Invalid id, %s expected %s", val, testId)
		}
		if key == "type" {
			assertTrue(val == string(as.ServiceType), "Invalid type, %s expected %s", val, as.ServiceType)
		}
		if key == "url" {
			assertTrue(val == testUrl, "Invalid url, %s expected %s", val, testUrl)
		}
		if key == "outbox" {
			assertTrue(val == testOutbox, "Invalid outbox url, %s expected %s", val, testOutbox)
		}
		if key == "inbox" {
			assertTrue(val == testInbox, "Invalid inbox url, %s expected %s", val, testInbox)
		}
		if key == "attributedTo" {
			assertTrue(val == testAuthor, "Invalid author, %s expected %s", val, testAuthor)
		}
	}
}
