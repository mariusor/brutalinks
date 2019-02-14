package tests

import (
	"fmt"
	as "github.com/go-ap/activitystreams"
	"os"
	"path"
	"testing"
)

var apiURL = os.Getenv("API_URL")

type collectionTestPairs map[string]collectionVal

var testPairs = collectionTestPairs{
	fmt.Sprintf("%s/actors", apiURL): {
		id:        fmt.Sprintf("%s/actors", apiURL),
		typ:       string(as.CollectionType),
		itemCount: 2,
		items: map[string]objectVal{
			fmt.Sprintf("%s/actors/eacff9dd", apiURL): {
				id:  fmt.Sprintf("%s/actors/eacff9dd", apiURL),
				typ: string(as.PersonType),
			},
			fmt.Sprintf("%s/actors/dc6f5f5b", apiURL): {
				id:  fmt.Sprintf("%s/actors/dc6f5f5b", apiURL),
				typ: string(as.PersonType),
			},
		},
	},
	fmt.Sprintf("%s/self/outbox", apiURL): {
		id:        fmt.Sprintf("%s/self/outbox", apiURL),
		typ:       string(as.OrderedCollectionType),
		itemCount: 1,
		items: map[string]objectVal{
			fmt.Sprintf("%s/actors/dc6f5f5b/outbox/162edb32", apiURL): {
				id:  fmt.Sprintf("%s/actors/dc6f5f5b/outbox/162edb32", apiURL),
				typ: string(as.CreateType),
			},
		},
	},
	fmt.Sprintf("%s/self/inbox", apiURL): {
		id:        fmt.Sprintf("%s/self/inbox", apiURL),
		typ:       string(as.OrderedCollectionType),
		itemCount: 1,
		items: map[string]objectVal{
			fmt.Sprintf("%s/actors/dc6f5f5b/outbox/162edb32", apiURL): {
				id:  fmt.Sprintf("%s/actors/dc6f5f5b/outbox/162edb32", apiURL),
				typ: string(as.CreateType),
			},
		},
	},
}

func Test_GETCollections(t *testing.T) {
	for k, col := range testPairs {
		t.Run(path.Base(k), func(t *testing.T) {
			errOnCollection(t)(k, col)
		})
	}
}
