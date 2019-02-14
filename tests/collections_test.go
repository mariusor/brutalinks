package tests

import (
	"fmt"
	as "github.com/go-ap/activitystreams"
	"os"
	"testing"
)

var apiURL = os.Getenv("API_URL")
var host = os.Getenv("HOSTNAME")

type collectionTestPairs map[string]collectionVal

var testPairs = collectionTestPairs{
	fmt.Sprintf("%s/actors", apiURL): {
		id:  fmt.Sprintf("%s/actors", apiURL),
		typ: string(as.CollectionType),
		first: &collectionVal{
			id: fmt.Sprintf("%s/actors?page=1", apiURL),
			// TODO(marius): fix actors collection pages
			//typ: string(as.CollectionPageType),
		},
		itemCount: 2,
		items: map[string]objectVal{
			fmt.Sprintf("%s/actors/eacff9dd", apiURL): {
				id:                fmt.Sprintf("%s/actors/eacff9dd", apiURL),
				typ:               string(as.PersonType),
				name:              "anonymous",
				preferredUsername: "anonymous",
				url:               fmt.Sprintf("http://%s/~anonymous", host),
				inbox: &collectionVal{
					id: fmt.Sprintf("%s/actors/eacff9dd/inbox", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					//typ: string(as.OrderedCollectionPageType),
				},
				outbox: &collectionVal{
					id: fmt.Sprintf("%s/actors/eacff9dd/outbox", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					//typ: string(as.OrderedCollectionPageType),
				},
				liked: &collectionVal{
					id: fmt.Sprintf("%s/actors/eacff9dd/liked", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					//typ: string(as.OrderedCollectionPageType),
				},
				score: 0,
			},
			fmt.Sprintf("%s/actors/dc6f5f5b", apiURL): {
				id:                fmt.Sprintf("%s/actors/dc6f5f5b", apiURL),
				typ:               string(as.PersonType),
				name:              "system",
				preferredUsername: "system",
				url:               fmt.Sprintf("http://%s/~system", host),
				inbox: &collectionVal{
					id: fmt.Sprintf("%s/actors/dc6f5f5b/inbox", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					//typ: string(as.OrderedCollectionType),
				},
				outbox: &collectionVal{
					id: fmt.Sprintf("%s/actors/dc6f5f5b/outbox", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					//typ: string(as.OrderedCollectionType),
				},
				liked: &collectionVal{
					id: fmt.Sprintf("%s/actors/dc6f5f5b/liked", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					//typ: string(as.OrderedCollectionType),
				},
				score: 0,
			},
		},
	},
	fmt.Sprintf("%s/self/inbox", apiURL): {
		id:  fmt.Sprintf("%s/self/inbox", apiURL),
		typ: string(as.OrderedCollectionType),
		first: &collectionVal{
			id: fmt.Sprintf("%s/self/inbox?maxItems=50&page=1", apiURL),
		},
		// TODO(marius): We need to fix the criteria for populating the inbox to
		//     verifying if the actor that submitted the activity is local or not
		itemCount: 1, // TODO(marius) :FIX_INBOX: this should be 0
	},
	fmt.Sprintf("%s/self/liked", apiURL): {
		id:  fmt.Sprintf("%s/self/liked", apiURL),
		typ: string(as.OrderedCollectionType),
		first: &collectionVal{
			id: fmt.Sprintf("%s/self/liked?maxItems=50&page=1", apiURL),
		},
		itemCount: 0,
	},
	fmt.Sprintf("%s/self/outbox", apiURL): {
		id:  fmt.Sprintf("%s/self/outbox", apiURL),
		typ: string(as.OrderedCollectionType),
		first: &collectionVal{
			id: fmt.Sprintf("%s/self/outbox?maxItems=50&page=1", apiURL),
		},
		itemCount: 1,
		items: map[string]objectVal{
			fmt.Sprintf("%s/actors/dc6f5f5b/outbox/162edb32", apiURL): {
				id:  fmt.Sprintf("%s/actors/dc6f5f5b/outbox/162edb32", apiURL),
				typ: string(as.CreateType),
				act: &objectVal{
					id: fmt.Sprintf("%s/actors/dc6f5f5b", apiURL),
				},
				obj: &objectVal{
					id:        fmt.Sprintf("%s/actors/dc6f5f5b/outbox/162edb32/object", apiURL),
					typ:       string(as.NoteType),
					name:      "about littr.me",
					url:       "/~system/162edb32",
					content:   "<p>This is a new attempt at the social news aggregator paradigm.<br/>It's based on the ActivityPub web specification and as such tries to leverage federation to prevent some of the pitfalls found in similar existing communities.</p>",
					mediaType: "text/html",
					author:    fmt.Sprintf("%s/actors/dc6f5f5b", apiURL),
					score:     0,
				},
			},
		},
	},
}

func Test_GETCollections(t *testing.T) {
	assertCollection := errOnCollection(t)

	for k, col := range testPairs {
		t.Run(k, func(t *testing.T) {
			assertCollection(k, col)
		})
	}
}
