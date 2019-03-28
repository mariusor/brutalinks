package tests

import (
	"fmt"
	as "github.com/go-ap/activitystreams"
	"net/http"
	"testing"
)

var Tests = testPairs{
	"C2S_Load": {
		{
			req: testReq{
				met: http.MethodGet,
				url: fmt.Sprintf("%s/self", apiURL),
			},
			res: testRes{
				code: http.StatusOK,
				val: objectVal{
					id:      fmt.Sprintf("%s/self", apiURL),
					typ:     string(as.ServiceType),
					name:    "127 0 0 3",
					summary: "Link aggregator inspired by reddit and hacker news using ActivityPub federation.",
					url:     fmt.Sprintf("http://%s", host),
					liked: &objectVal{
						id:        fmt.Sprintf("%s/self/liked", apiURL),
						typ:       string(as.OrderedCollectionType),
						itemCount: 0,
					},
					inbox: &objectVal{
						id:  fmt.Sprintf("%s/self/inbox", apiURL),
						typ: string(as.OrderedCollectionType),
						first: &objectVal{
							id: fmt.Sprintf("%s/self/inbox?maxItems=50&page=1", apiURL),
						},
						// TODO(marius): We need to fix the criteria for populating the inbox to
						//     verifying if the actor that submitted the activity is local or not
						itemCount: 1, // TODO(marius): :FIX_INBOX: this should be 0
					},
					outbox: &objectVal{
						id:  fmt.Sprintf("%s/self/outbox", apiURL),
						typ: string(as.OrderedCollectionType),
						first: &objectVal{
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
					following: &objectVal{
						id:  fmt.Sprintf("%s/self/following", apiURL),
						typ: string(as.OrderedCollectionType),
						first: &objectVal{
							id: fmt.Sprintf("%s/self/following?maxItems=50&page=1", apiURL),
							//typ: string(as.CollectionPageType),
						},
						itemCount: 3,
						items: map[string]objectVal{
							"self/following/eacff9ddf379bd9fc8274c5a9f4cae08": {
								id:                fmt.Sprintf("%s/self/following/eacff9ddf379bd9fc8274c5a9f4cae08", apiURL),
								typ:               string(as.PersonType),
								name:              "anonymous",
								preferredUsername: "anonymous",
								url:               fmt.Sprintf("http://%s/~anonymous", host),
								inbox: &objectVal{
									id:  fmt.Sprintf("%s/self/following/eacff9ddf379bd9fc8274c5a9f4cae08/inbox", apiURL),
									typ: string(as.OrderedCollectionType),
								},
								outbox: &objectVal{
									id: fmt.Sprintf("%s/self/following/eacff9ddf379bd9fc8274c5a9f4cae08/outbox", apiURL),
									// TODO(marius): Fix different page id when dereferenced vs. in parent collection
									typ: string(as.OrderedCollectionType),
								},
								liked: &objectVal{
									id: fmt.Sprintf("%s/self/following/eacff9ddf379bd9fc8274c5a9f4cae08/liked", apiURL),
									// TODO(marius): Fix different page id when dereferenced vs. in parent collection
									typ: string(as.OrderedCollectionType),
								},
								score: 0,
							},
							"self/following/dc6f5f5bf55bc1073715c98c69fa7ca8": {
								id:                fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8", apiURL),
								typ:               string(as.PersonType),
								name:              "system",
								preferredUsername: "system",
								url:               fmt.Sprintf("http://%s/~system", host),
								inbox: &objectVal{
									id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/inbox", apiURL),
									// TODO(marius): Fix different page id when dereferenced vs. in parent collection
									typ: string(as.OrderedCollectionType),
								},
								outbox: &objectVal{
									id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox", apiURL),
									// TODO(marius): Fix different page id when dereferenced vs. in parent collection
									typ: string(as.OrderedCollectionType),
								},
								liked: &objectVal{
									id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/liked", apiURL),
									//TODO(marius): Fix different page id when dereferenced vs. in parent collection
									typ: string(as.OrderedCollectionType),
								},
								score: 0,
							},
							"self/following/" + testActorHash: {
								id:                fmt.Sprintf("%s/self/following/%s", apiURL, testActorHash),
								typ:               string(as.PersonType),
								name:              "johndoe",
								preferredUsername: "johndoe",
								url:               fmt.Sprintf("http://%s/~johndoe", host),
								inbox: &objectVal{
									id: fmt.Sprintf("%s/self/following/%s/inbox", apiURL, testActorHash),
									// TODO(marius): Fix different page id when dereferenced vs. in parent collection
									typ: string(as.OrderedCollectionType),
								},
								outbox: &objectVal{
									id: fmt.Sprintf("%s/self/following/%s/outbox", apiURL, testActorHash),
									// TODO(marius): Fix different page id when dereferenced vs. in parent collection
									typ: string(as.OrderedCollectionType),
								},
								liked: &objectVal{
									id: fmt.Sprintf("%s/self/following/%s/liked", apiURL, testActorHash),
									// TODO(marius): Fix different page id when dereferenced vs. in parent collection
									typ: string(as.OrderedCollectionType),
								},
								score: 0,
							},
						},
					},
					author: "https://github.com/mariusor",
				},
			},
		},
	},
	"C2S_Like": {{
		req: testReq{
			met:     http.MethodPost,
			url:     outboxURL,
			account: &defaultTestAccount,
			body: fmt.Sprintf(`{
  "type": "Like",
  "actor": "%s/self/following/%s",
  "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object"
}`, apiURL, testActorHash, apiURL),
		},
		res: testRes{
			code: http.StatusCreated,
			val: objectVal{
				id:  fmt.Sprintf("%s/self/following/%s/liked/162edb32c80d0e6dd3114fbb59d6273b", apiURL, testActorHash),
				typ: string(as.LikeType),
				obj: &objectVal{
					id:     fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object", apiURL),
					author: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8", apiURL),
				},
			},
		},
	}},
	"C2S_AnonymousLike": {{
		req: testReq{
			met: http.MethodPost,
			url: outboxURL,
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
	"C2S_Dislike": {{
		req: testReq{
			met:     http.MethodPost,
			url:     outboxURL,
			account: &defaultTestAccount,
			body: fmt.Sprintf(`{
  "type": "Dislike",
  "actor": "%s/self/following/%s",
  "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object"
}`, apiURL, testActorHash, apiURL),
		},
		res: testRes{
			code: http.StatusCreated,
			val: objectVal{
				id:  fmt.Sprintf("%s/self/following/%s/liked/162edb32c80d0e6dd3114fbb59d6273b", apiURL, testActorHash),
				typ: string(as.DislikeType),
				obj: &objectVal{
					id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object", apiURL),
				},
			},
		},
	}},
	"C2S_AnonymousDislike": {{
		req: testReq{
			met: http.MethodPost,
			url: outboxURL,
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
	"C2S_UndoDislike": {
		{
			req: testReq{
				met:     http.MethodPost,
				url:     outboxURL,
				account: &defaultTestAccount,
				body: fmt.Sprintf(`{
  "type": "Dislike",
  "actor": "%s/self/following/%s",
  "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object"
}`, apiURL, testActorHash, apiURL),
			},
			res: testRes{
				code: http.StatusCreated,
				val: objectVal{
					id:  fmt.Sprintf("%s/self/following/%s/liked/162edb32c80d0e6dd3114fbb59d6273b", apiURL, testActorHash),
					typ: string(as.DislikeType),
					obj: &objectVal{
						id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object", apiURL),
					},
				},
			},
		},
		{
			req: testReq{
				met:     http.MethodPost,
				url:     outboxURL,
				account: &defaultTestAccount,
				body: fmt.Sprintf(`{
  "type": "Dislike",
  "actor": "%s/self/following/%s",
  "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object"
}`, apiURL, testActorHash, apiURL),
			},
			res: testRes{
				code: http.StatusCreated,
				val: objectVal{
					id:  fmt.Sprintf("%s/self/following/%s/liked/162edb32c80d0e6dd3114fbb59d6273b", apiURL, testActorHash),
					typ: string(as.UndoType),
					obj: &objectVal{
						id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object", apiURL),
					},
				},
			},
		},
	},
	"C2S_UndoLike": {
		{
			req: testReq{
				met:     http.MethodPost,
				url:     outboxURL,
				account: &defaultTestAccount,
				body: fmt.Sprintf(`{
  "type": "Like",
  "actor": "%s/self/following/%s",
  "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object"
}`, apiURL, testActorHash, apiURL),
			},
			res: testRes{
				code: http.StatusCreated,
				val: objectVal{
					id:  fmt.Sprintf("%s/self/following/%s/liked/162edb32c80d0e6dd3114fbb59d6273b", apiURL, testActorHash),
					typ: string(as.LikeType),
					obj: &objectVal{
						id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object", apiURL),
					},
				},
			},
		},
		{
			req: testReq{
				met:     http.MethodPost,
				url:     outboxURL,
				account: &defaultTestAccount,
				body: fmt.Sprintf(`{
  "type": "Like",
  "actor": "%s/self/following/%s",
  "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object"
}`, apiURL, testActorHash, apiURL),
			},
			res: testRes{
				code: http.StatusCreated,
				val: objectVal{
					id:  fmt.Sprintf("%s/self/following/%s/liked/162edb32c80d0e6dd3114fbb59d6273b", apiURL, testActorHash),
					typ: string(as.UndoType),
					obj: &objectVal{
						id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object", apiURL),
					},
				},
			},
		},
	},
	"C2S_UndoLike#2": {
		{
			req: testReq{
				met:     http.MethodPost,
				url:     outboxURL,
				account: &defaultTestAccount,
				body: fmt.Sprintf(`{
  "type": "Like",
  "actor": "%s/self/following/%s",
  "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object"
}`, apiURL, testActorHash, apiURL),
			},
			res: testRes{
				code: http.StatusCreated,
				val: objectVal{
					id:  fmt.Sprintf("%s/self/following/%s/liked/162edb32c80d0e6dd3114fbb59d6273b", apiURL, testActorHash),
					typ: string(as.LikeType),
					obj: &objectVal{
						id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object", apiURL),
					},
				},
			},
		},
		{
			req: testReq{
				met:     http.MethodPost,
				url:     outboxURL,
				account: &defaultTestAccount,
				body: fmt.Sprintf(`{
  "type": "Undo",
  "actor": "%s/self/following/%s",
  "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object"
}`, apiURL, testActorHash, apiURL),
			},
			res: testRes{
				code: http.StatusCreated,
				val: objectVal{
					id:  fmt.Sprintf("%s/self/following/%s/liked/162edb32c80d0e6dd3114fbb59d6273b", apiURL, testActorHash),
					typ: string(as.UndoType),
					obj: &objectVal{
						id: fmt.Sprintf("%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object", apiURL),
					},
				},
			},
		},
	},
	"C2S_Create": {{
		req: testReq{
			met:     http.MethodPost,
			url:     outboxURL,
			account: &defaultTestAccount,
			body: fmt.Sprintf(`{
"type": "Create",
"actor": "%s/self/following/%s",
"to": ["%s/self/outbox"],
"object": {
  "type": "Note",
  "inReplyTo": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b",
  "content": "<p>Hello world!</p>"
}
}`, apiURL, testActorHash, apiURL, apiURL),
		},
		res: testRes{
			code: http.StatusCreated,
			val: objectVal{
				typ: string(as.CreateType),
				obj: &objectVal{
					author:  fmt.Sprintf("%s/self/following/%s", apiURL, testActorHash),
					typ:     string(as.NoteType),
					content: "<p>Hello world!</p>",
				},
			},
		},
	}},
	"C2S_AnonymousCreate": {{
		req: testReq{
			met: http.MethodPost,
			url: outboxURL,
			body: fmt.Sprintf(`{
 "type": "Create",
 "actor": "%s/self/following/%s",
 "to": ["%s/self/outbox"],
 "object": {
   "type": "Note",
   "inReplyTo": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b",
   "content": "<p>Hello world!</p>"
 }
}`, apiURL, testActorHash, apiURL, apiURL),
		},
		res: testRes{
			code: http.StatusForbidden,
		},
	}},
	"C2S_Delete": {{
		req: testReq{
			met:     http.MethodPost,
			url:     outboxURL,
			account: &defaultTestAccount,
			body: fmt.Sprintf(`{
"type": "Delete",
"actor": "%s/self/following/%s",
"to": ["%s/self/outbox"],
"object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b"
}`, apiURL, testActorHash, apiURL, apiURL),
		},
		res: testRes{
			code: http.StatusCreated,
			val: objectVal{
				typ: string(as.DeleteType),
				obj: &objectVal{
					typ: string(as.TombstoneType),
				},
			},
		},
	}},
	"C2S_AnonymousDelete": {{
		req: testReq{
			met: http.MethodPost,
			url: outboxURL,
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

func Test_Integration(t *testing.T) {
	for typ, tests := range Tests {
		resetDB(t, true)
		for _, test := range tests {
			lbl := fmt.Sprintf("%s:%s:%s:%s", typ, test.req.met, test.res.val.typ, test.req.url)
			t.Run(lbl, func(t *testing.T) {
				errOnRequest(t)(test)
			})
		}
	}
}
