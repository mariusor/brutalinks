package api

import (
	"net/http"
	"strings"
)

// handleMain serves /.webfinger/ request
func HandleWebFinger(w http.ResponseWriter, r *http.Request) {
	typ, res := func(ar []string) (string, string) {
		if len(ar) != 2 {
			return "", ""
		}

		return ar[0], ar[1]
	}(strings.Split(r.URL.Query()["resource"][0], ":"))

	if typ == "" || res == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("{}"))
		return
	}

	d := `{"subject": "` + typ + `:` + res + `","links": [{"rel": "self","type": "application/activity+json","href": "http://littr.git/actor"}]}`
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(d))
}
