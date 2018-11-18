package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mariusor/littr.go/app/frontend"

	"github.com/mariusor/littr.go/app"

	"github.com/juju/errors"
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
				Template: fmt.Sprintf("%s/.well-known/webfinger?resource={uri}", app.Instance.BaseURL),
			},
		},
	}
	dat, _ := json.Marshal(hm)

	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

// HandleWebFinger serves /.well-known/webfinger/ request
func (h handler)HandleWebFinger(w http.ResponseWriter, r *http.Request) {
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

	handle := strings.Replace(res, fmt.Sprintf("@%s", app.Instance.HostName), "", 1)

	val := r.Context().Value(app.RepositoryCtxtKey)
	AcctLoader, ok := val.(app.CanLoadAccounts)
	if !ok {
		err := errors.New("could not load account repository from Context")
		h.logger.Error(err.Error())
		h.HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	a, err := AcctLoader.LoadAccount(app.LoadAccountsFilter{Handle: []string{handle}})
	if err != nil {
		err := errors.New("resource not found")
		h.logger.Error(err.Error())
		h.HandleError(w, r, http.StatusNotFound, err)
		return
	}

	wf := webfinger{
		Aliases: []string{
			fmt.Sprintf("%s/%s", ActorsURL, a.Hash),
			fmt.Sprintf("%s/%s", ActorsURL, a.Handle),
		},
		Subject: typ + ":" + res,
		Links: []link{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: fmt.Sprintf("%s/%s", ActorsURL, a.Hash),
			},
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "application/activity+json",
				Href: fmt.Sprintf("%s/%s", ActorsURL, a.Hash),
			},
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: frontend.AccountPermaLink(a),
			},
		},
	}

	dat, _ := json.Marshal(wf)
	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}
