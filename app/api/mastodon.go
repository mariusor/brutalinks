package api

import (
	"encoding/json"
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"math"
	"net/http"
)

// GET /api/v1/instance
// In order to be compatible with Mastodon
func ShowInstance(w http.ResponseWriter, r *http.Request) {
	ifErr := func(err ...error) {
		if err != nil && len(err) > 0 && err[0] != nil {
			HandleError(w, r, http.StatusInternalServerError, err...)
			return
		}
	}

	inf, err := db.Config.LoadInfo()
	ifErr(err)

	u, err := db.Config.LoadAccounts(app.LoadAccountsFilter{
		MaxItems: math.MaxInt64,
	})
	ifErr(err)
	i, err := db.Config.LoadItems(app.LoadItemsFilter{
		MaxItems: math.MaxInt64,
	})
	ifErr(err)

	d := app.Desc{}
	d.Stats.DomainCount = 1
	d.Stats.UserCount = len(u)
	d.Stats.StatusCount = len(i)
	d.URI = inf.URI
	d.Title = inf.Title
	d.Email = inf.Email
	d.Lang = inf.Languages
	d.Thumbnail = inf.Thumbnail
	d.Description = inf.Summary
	d.Version = fmt.Sprintf("2.5.0 compatible (%s %s)", app.Instance.HostName, inf.Version)

	data, err := json.Marshal(d)
	ifErr(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/v1/instance/peers
// In order to be compatible with Mastodon
func ShowPeers(w http.ResponseWriter, r *http.Request) {
	em := []string{app.Instance.HostName}
	data, _ := json.Marshal(em)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

type activity struct {
	Week         int `json:"week"`
	Statuses     int `json:"statuses"`
	Logins       int `json:"logins"`
	Registration int `json:"registrations"`
}

// ShowActivity
// GET /api/v1/instance/activity
// In order to be compatible with Mastodon
func ShowActivity(w http.ResponseWriter, r *http.Request) {
	em := make([]activity, 0)
	data, _ := json.Marshal(em)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
