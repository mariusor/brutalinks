package tests

import (
	"fmt"
	"net/http"
	"testing"
)

var metalheadAcct = testAccount{
	id:         "https://metalhead.club/users/mariusor",
	Handle:     "mariusor",
	publicKey:  key.Public(),
	privateKey: key,
}

var S2STests = testPairs{
	"Follow_SelfService": {{
		req: testReq{
			met:     http.MethodPost,
			url:     inboxURL,
			account: &metalheadAcct,
			body: fmt.Sprintf(`{
 "type": "Follow",
 "actor": "https://metalhead.club/users/mariusor",
 "to": ["%s/self"],
}`, apiURL),
		},
		res: testRes{
			code: http.StatusForbidden,
		},
	}},
}

func Test_S2SRequests(t *testing.T) {
	testSuite(t, S2STests)
}
