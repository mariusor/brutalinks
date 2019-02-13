package tests

import (
	"fmt"
	as "github.com/go-ap/activitystreams"
	"os"
	"testing"
)

var apiURL = os.Getenv("API_URL")

type collectionTestPairs map[string]collectionVal
var testPairs = collectionTestPairs{
	fmt.Sprintf("%s/actors", apiURL):
	collectionVal{
		iri: fmt.Sprintf("%s/actors", apiURL),
		typ: string(as.CollectionType),
		itemCount: 2,
	},
	fmt.Sprintf("%s/self/outbox", apiURL):
	collectionVal{
		iri: fmt.Sprintf("%s/self/outbox", apiURL),
		typ: string(as.OrderedCollectionType),
		itemCount: 1,
	},
	fmt.Sprintf("%s/self/inbox", apiURL):
	collectionVal{
		iri: fmt.Sprintf("%s/self/inbox", apiURL),
		typ: string(as.OrderedCollectionType),
		itemCount: 1,
	},
}

func Test_GETCollections(t *testing.T) {
	for k, col := range testPairs {
		t.Run(k, func(t *testing.T) {
			errOnCollection(t)(k, col)
		})
	}
}
