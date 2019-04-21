package tests

import (
	"fmt"
	"net/http"
	"testing"
)

var S2STests = testPairs{
	"S2S_Follow": {{
		req: testReq{
			met: http.MethodPost,
			url: inboxURL,
			body: fmt.Sprintf(`{
 "type": "Follow",
 "actor": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8",
 "to": ["%s/self/inbox"],
 "object": "%s/self/following/dc6f5f5bf55bc1073715c98c69fa7ca8/outbox/162edb32c80d0e6dd3114fbb59d6273b"
}`, apiURL, apiURL, apiURL),
		},
		res: testRes{
			code: http.StatusForbidden,
		},
	}},
}

func Test_S2SRequests(t *testing.T) {
	testSuite(t, S2STests)
}
