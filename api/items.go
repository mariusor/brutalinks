package api

import (
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"

	ap "github.com/mariusor/activitypub.go/activitypub"
	j "github.com/mariusor/activitypub.go/jsonld"
)

// GET /api/accounts/fy_credentials
func HandleItem(w http.ResponseWriter, r *http.Request) {
	// verify signature header:
	// Signature: keyId="https://my-example.com/actor#main-key",headers="(request-target) host date",signature="..."

	c := ap.Create{}
	var body []byte
	var err error
	if r != nil {
		defer r.Body.Close()

		if body, err = ioutil.ReadAll(r.Body); err != nil {
			logrus.WithFields(logrus.Fields{}).Errorf("request body read error: %s", err)
		}
		if err := j.Unmarshal(body, &c); err != nil {
			logrus.WithFields(logrus.Fields{}).Errorf("json-ld unmarshal error: %s", err)
		}
	}
	w.Header().Set("Content-Type", "application/ld+json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "ok"}`))
}
