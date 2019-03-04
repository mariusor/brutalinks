package tests

import (
	"fmt"
	as "github.com/go-ap/activitystreams"
	"net/http"
	"testing"
)

type getTest map[string]collectionVal
type postTest map[string][]postTestVal

var defaultCollectionTestPairs = getTest{
	"self/following": {
		id:  fmt.Sprintf("%s/self/following", apiURL),
		typ: string(as.CollectionType),
		first: &collectionVal{
			id: fmt.Sprintf("%s/self/following?page=1", apiURL),
			// TODO(marius): fix actors collection pages
			//typ: string(as.CollectionPageType),
		},
		itemCount: 2,
		items: map[string]objectVal{
			"self/following/eacff9ddf379bd9fc8274c5a9f4cae08": {
				id:                fmt.Sprintf("%s/self/following/eacff9ddf379bd9fc8274c5a9f4cae08", apiURL),
				typ:               string(as.PersonType),
				name:              "anonymous",
				preferredUsername: "anonymous",
				url:               fmt.Sprintf("http://%s/~anonymous", host),
				inbox: &collectionVal{
					id: fmt.Sprintf("%s/self/following/eacff9ddf379bd9fc8274c5a9f4cae08/inbox", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					//typ: string(as.OrderedCollectionPageType),
				},
				outbox: &collectionVal{
					id: fmt.Sprintf("%s/self/following/eacff9ddf379bd9fc8274c5a9f4cae08/outbox", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					//typ: string(as.OrderedCollectionPageType),
				},
				liked: &collectionVal{
					id: fmt.Sprintf("%s/self/following/eacff9ddf379bd9fc8274c5a9f4cae08/liked", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					//typ: string(as.OrderedCollectionPageType),
				},
				score: 0,
			},
			"self/following/dc6f5f5bf55bc1073715c98c69fa7ca8": {
				id:                fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8", apiURL),
				typ:               string(as.PersonType),
				name:              "system",
				preferredUsername: "system",
				url:               fmt.Sprintf("http://%s/~system", host),
				inbox: &collectionVal{
					id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/inbox", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					typ: string(as.OrderedCollectionType),
				},
				outbox: &collectionVal{
					id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					typ: string(as.OrderedCollectionType),
				},
				liked: &collectionVal{
					id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/liked", apiURL),
					// TODO(marius): Fix different page id when dereferenced vs. in parent collection
					typ: string(as.OrderedCollectionType),
				},
				score: 0,
			},
		},
	},
	"self/inbox": {
		id:  fmt.Sprintf("%s/self/inbox", apiURL),
		typ: string(as.OrderedCollectionType),
		first: &collectionVal{
			id: fmt.Sprintf("%s/self/inbox?maxItems=50&page=1", apiURL),
		},
		// TODO(marius): We need to fix the criteria for populating the inbox to
		//     verifying if the actor that submitted the activity is local or not
		itemCount: 1, // TODO(marius): :FIX_INBOX: this should be 0
	},
	"self/liked": {
		id:        fmt.Sprintf("%s/self/liked", apiURL),
		typ:       string(as.OrderedCollectionType),
		itemCount: 0,
	},
	"self/outbox": {
		id:  fmt.Sprintf("%s/self/outbox", apiURL),
		typ: string(as.OrderedCollectionType),
		first: &collectionVal{
			id: fmt.Sprintf("%s/self/outbox?maxItems=50&page=1", apiURL),
		},
		itemCount: 1,
		items: map[string]objectVal{
			"self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b": {
				id:  fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b", apiURL),
				typ: string(as.CreateType),
				act: &objectVal{
					id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8", apiURL),
				},
				obj: &objectVal{
					id:        fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object", apiURL),
					typ:       string(as.NoteType),
					name:      "about littr.me",
					url:       "/~system/162edb32c8",
					content:   "<p>This is a new attempt at the social news aggregator paradigm.<br/>It's based on the ActivityPub web specification and as such tries to leverage federation to prevent some of the pitfalls found in similar existing communities.</p>",
					mediaType: "text/html",
					author:    fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8", apiURL),
					score:     0,
				},
			},
		},
	},
}

var c2sTestPairs = postTest{
	"AnonymousLike": {{
		req: testReq{
			met: http.MethodPost,
			body: fmt.Sprintf(`{
    "type": "Like",
    "actor": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8",
    "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object"
}`, apiURL, apiURL),
		},
		res: testRes{
			code: http.StatusForbidden,
		},
	}},
	"AnonymousDislike": {{
		req: testReq{
			met: http.MethodPost,
			body: fmt.Sprintf(`{
    "type": "Dislike",
    "actor": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8",
    "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object"
}`, apiURL, apiURL),
		},
		res: testRes{
			code: http.StatusForbidden,
		},
	}},
	"AnonymousCreate": {{
		req: testReq{
			met: http.MethodPost,
			body: fmt.Sprintf(`{
  "type": "Create",
  "actor": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8",
  "to": ["%s/self/outbox"],
  "object": {
    "type": "Note",
    "inReplyTo": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b",
    "content": "<p>Hello world!</p>"
  }
}`, apiURL, apiURL, apiURL),
		},
		res: testRes{
			code: http.StatusForbidden,
		},
	}},
	"AnonymousDelete": {{
		req: testReq{
			met: http.MethodPost,
			body: fmt.Sprintf(`{
  "type": "Delete",
  "actor": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8",
  "to": ["%s/self/outbox"],
  "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b"
}`, apiURL, apiURL, apiURL),
		},
		res: testRes{
			code: http.StatusForbidden,
		},
	}},
}

var s2sTestPairs = postTest{}

func Test_GET(t *testing.T) {
	assertCollection := errOnCollection(t)
	resetDB(t)
	for k, col := range defaultCollectionTestPairs {
		t.Run(k, func(t *testing.T) {
			t.Parallel()
			assertCollection(fmt.Sprintf("%s/%s", apiURL, k), col)
		})
	}
}

func Test_POST_Outbox(t *testing.T) {
	assertPost := errOnPostRequest(t)
	for typ, test := range c2sTestPairs {
		t.Run("Activity_"+typ, func(t *testing.T) {
			resetDB(t)
			assertPost(test)
		})
	}
}
