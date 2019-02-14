package tests

import (
	"fmt"
	as "github.com/go-ap/activitystreams"
	"os"
	"testing"
)

func Test_GETNodeInfo(t *testing.T) {
	assertReq := errOnGetRequest(t)
	assertMapKey := errOnMapProp(t)

	apiURL := os.Getenv("API_URL")
	host := os.Getenv("HOSTNAME")

	testId := fmt.Sprintf("%s/self", apiURL)
	testUrl := fmt.Sprintf("http://%s", host)
	testOutbox := fmt.Sprintf("%s/outbox", testId)
	testInbox := fmt.Sprintf("%s/inbox", testId)
	testAuthor := "https://github.com/mariusor"

	url := fmt.Sprintf("%s/self", apiURL)
	test := assertReq(url)

	assertMapKey(test, "id", testId)
	assertMapKey(test, "type", string(as.ServiceType))
	assertMapKey(test, "url", testUrl)
	assertMapKey(test, "outbox", testOutbox)
	assertMapKey(test, "inbox", testInbox)
	assertMapKey(test, "attributedTo", testAuthor)
}
