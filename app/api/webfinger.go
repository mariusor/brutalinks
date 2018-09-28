package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

type link struct {
	Rel      string `json:"rel,omitempty"`
	Type     string `json:"type,omitempty"`
	Href     string `json:"href,omitempty"`
	Template string `json:"template,omitempty"`
}

type webfinger struct {
	Subject string   `json:"subject"`
	Aliases []string `json:"aliases"`
	Links   []link   `json:"links"`
}

// HandleHostMeta serves /.well-known/host-meta
func HandleHostMeta(w http.ResponseWriter, r *http.Request) {

	hm := webfinger{
		Links: []link{
			{
				Rel:      "lrdd",
				Type:     "application/xrd+json",
				Template: fmt.Sprintf("https://%s/.well-known/webfinger?resource={uri}", "littr.me"),
			},
		},
	}
	dat, _ := json.Marshal(hm)
	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
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

	handle := strings.Replace(res, "@littr.me", "", 1)

	val := r.Context().Value(models.RepositoryCtxtKey)
	AcctLoader, ok := val.(models.CanLoadAccounts)
	if !ok {
		err := errors.New("could not load account repository from Context")
		Logger.WithFields(log.Fields{}).Error(err)
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	a, err := AcctLoader.LoadAccount(models.LoadAccountsFilter{Handle: []string{handle}})
	if err != nil {
		err := errors.New("resource not found")
		Logger.WithFields(log.Fields{}).Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}

	wf := webfinger{
		Aliases: []string{
			fmt.Sprintf("https://%s/api/accounts/%s", "littr.me", a.Handle),
		},
		Subject: typ + ":" + res,
		Links: []link{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: fmt.Sprintf("https://%s/api/accounts/%s", "littr.me", a.Hash),
			},
			//{
			//	Rel:  "http://webfinger.net/rel/profile-page",
			//	Href: fmt.Sprintf("https://%s/api/accounts/%s", "littr.me", a.Hash),
			//},
		},
	}

	dat, _ := json.Marshal(wf)
	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}
