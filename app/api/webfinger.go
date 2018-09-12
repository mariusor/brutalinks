package api

import (
	"fmt"
	"net/http"
	"strings"
)

// HandleHostMeta serves /.well-known/host-meta
func HandleHostMeta(w http.ResponseWriter, r *http.Request) {

	d := fmt.Sprintf(`{ "links": [{ "rel": "lrdd", "type": "application/xrd+json", "template":"https://%s/.well-known/webfinger?resource={uri}" }] }`, "littr.me")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(d))
}

// HandleWebFinger serves /.well-known/webfinger/ request
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

	acct := strings.Replace(res, "@littr.me", "", 1)

	d := fmt.Sprintf(`{"subject": "`+typ+`:`+res+`","links": [{"rel": "self","type": "application/activity+json","href": "https://%s/api/accounts/%s"}]}`, "littr.me", acct)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(d))
}
