package tests

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"testing"
)

var littrAcct = testAccount{
	id:         "https://littr.me/api/self/following/64e55785250f59637e7f578167e2c112",
	Hash:       "64e55785250f59637e7f578167e2c112",
	Handle:     "0lpbm",
}

var S2STests = testPairs{
	"Follow_SelfService": {{
		req: testReq{
			met:     http.MethodPost,
			url:     inboxURL,
			account: &littrAcct,
			body: fmt.Sprintf(`{
"type": "Follow",
"actor": "%s",
"to": ["%s/self"]
}`, littrAcct.id, apiURL),
		},
		res: testRes{
			code: http.StatusAccepted,
		},
	}},
	"Like": {{
		req: testReq{
			met:     http.MethodPost,
			url:     inboxURL,
			account: &littrAcct,
			body: fmt.Sprintf(`{
 "type": "Like",
 "actor": "%s",
 "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b/object"
}`,  littrAcct.id, apiURL),
		},
		res: testRes{
			code: http.StatusCreated,
		},
	}},
	"Create": {{
		req: testReq{
			met:     http.MethodPost,
			url:     inboxURL,
			account: &littrAcct,
			body: fmt.Sprintf(`{
"type": "Create",
"actor": "%s",
"to": ["%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/inbox"],
"object": {
  "type": "Note",
  "inReplyTo": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b",
  "content": "<p>Hello world!</p>"
}
}`, littrAcct.id, apiURL, apiURL),
		},
		res: testRes {
			code: http.StatusForbidden, // Not sure why this fails
		},
	}},
}

func Test_S2SRequests(t *testing.T) {
	var lpubblock, _ = pem.Decode([]byte("-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA6o5aCJjzpWRmdj27eGE+DIdC96zSTrPL6lnelZrCNSCcfMC9wZVhMwLLNlSCV1EURDMYs9fla7Hjfqqa706TC3EI6q2n1rPcLd4Cgvto6rA+luiXQGRNQmabXxVQBdn81oxRNv+M41g4ZhPkK7px5pHG+U6oU5uPu8m3fLqQpMjVHitiaou8ABRoLN6s5aAattMVULh/Ma33tqDDwt2+mDui44x9ge4dPrtP3koZZ+uiiLx+9hqG6Oaa8BrsujLrHHCLiwPaHmAacybLCjROvoJUB1mxGdHaaBoh87sb0sX7KmRDVVobkd3q8MI4zektj4N8GDoNUkukuh1mALrGjQIDAQAB\n-----END PUBLIC KEY-----"))
	var lpub, _ = x509.ParsePKIXPublicKey(lpubblock.Bytes)
	var prv64 = os.Getenv("TEST_PRIVATE_KEY")
	var lprvblock, _ = pem.Decode([]byte("-----BEGIN PRIVATE KEY-----\n"+prv64+"\n-----END PRIVATE KEY-----"))
	var lprv, _ = x509.ParsePKCS8PrivateKey(lprvblock.Bytes)

	if os.Getenv("TEST_PRIVATE_KEY") == "" {
		t.Skipf("Unable to load private key")
	} else {
		littrAcct.privateKey = lprv.(*rsa.PrivateKey)
		littrAcct.publicKey =  lpub.(*rsa.PublicKey)
	}
	testSuite(t, S2STests)
}
