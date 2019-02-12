package api

import (
	"encoding/json"
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"github.com/writeas/go-nodeinfo"
	"math"
	"net/http"
)

type NodeInfoResolver struct{}

func (r NodeInfoResolver) IsOpenRegistration() (bool, error) {
	return false, nil
}

func (r NodeInfoResolver) Usage() (nodeinfo.Usage, error) {
	//inf, err := db.Config.LoadInfo()
	//ifErr(err)

	us, _, _ := db.Config.LoadAccounts(app.LoadAccountsFilter{
		MaxItems: math.MaxInt64,
	})
	//ifErr(err)
	i, _, _ := db.Config.LoadItems(app.LoadItemsFilter{
		MaxItems: math.MaxInt64,
	})
	//ifErr(err)

	u := nodeinfo.Usage{
		Users: nodeinfo.UsageUsers{
			Total: len(us),
		},
		LocalPosts: len(i),
	}
	return u, nil
}

// GET /api/v1/instance
// In order to be compatible with Mastodon
func (h handler) ShowInstance(w http.ResponseWriter, r *http.Request) {
	ifErr := func(err ...error) {
		if err != nil && len(err) > 0 && err[0] != nil {
			h.HandleError(w, r, err...)
			return
		}
	}

	inf, err := db.Config.LoadInfo()
	ifErr(err)

	_, uCount, err := db.Config.LoadAccounts(app.LoadAccountsFilter{})
	ifErr(err)
	_, iCount, err := db.Config.LoadItems(app.LoadItemsFilter{})
	ifErr(err)

	d := app.Desc{}
	d.Stats.DomainCount = 1
	d.Stats.UserCount = uCount
	d.Stats.StatusCount = iCount
	d.URI = inf.URI
	d.Title = inf.Title
	d.Email = inf.Email
	d.Lang = inf.Languages
	d.Thumbnail = inf.Thumbnail
	d.Description = inf.Summary
	d.Version = fmt.Sprintf("%s %s", app.Instance.HostName, inf.Version)

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
