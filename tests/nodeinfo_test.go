package tests

import (
	"fmt"
	as "github.com/go-ap/activitystreams"
	"os"
	"testing"
)

func Test_GETNodeInfo(t *testing.T) {
	assertReq := errOnGetRequest(t)
	assertObject := errOnObjectProperties(t)

	apiURL := os.Getenv("API_URL")
	host := os.Getenv("HOSTNAME")

	url := fmt.Sprintf("%s/self", apiURL)
	test := assertReq(url)

	assertObject(test, objectVal{
		id: fmt.Sprintf("%s/self", apiURL),
		typ: string(as.ServiceType),
		name: "127 0 0 3",
		summary: "Link aggregator inspired by reddit and hacker news using ActivityPub federation.",
		url: fmt.Sprintf("http://%s", host),
		inbox: &collectionVal{
			id: fmt.Sprintf("%s/self/inbox", apiURL),
		},
		outbox:  &collectionVal{
			id: fmt.Sprintf("%s/self/outbox", apiURL),
		},
		author: "https://github.com/mariusor",
	})
}
