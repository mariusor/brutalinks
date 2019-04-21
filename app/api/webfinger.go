package api

import (
	"encoding/json"
	"fmt"
	"github.com/mariusor/littr.go/app/db"
	"github.com/writeas/go-nodeinfo"
	"math"
	"net/http"
	"strings"

	"github.com/mariusor/littr.go/app"

	"github.com/mariusor/littr.go/internal/errors"
)

type link struct {
	Rel      string `json:"rel,omitempty"`
	Type     string `json:"type,omitempty"`
	Href     string `json:"href,omitempty"`
	Template string `json:"template,omitempty"`
}

type node struct {
	Subject string   `json:"subject"`
	Aliases []string `json:"aliases"`
	Links   []link   `json:"links"`
}

type NodeInfoResolver struct{}

func (r NodeInfoResolver) IsOpenRegistration() (bool, error) {
	return false, nil
}

func (r NodeInfoResolver) Usage() (nodeinfo.Usage, error) {
	us, _, _ := db.Config.LoadAccounts(app.Filters{
			LoadAccountsFilter: app.LoadAccountsFilter{
			//IRI:  app.Instance.APIURL,
			Deleted: []bool{false},
		},
		MaxItems: math.MaxInt64,
	})

	posts, _, _ := db.Config.LoadItems(app.Filters{
		LoadItemsFilter:app.LoadItemsFilter{
			Deleted: []bool{false},
			Context: []string{"0"},
		},
		MaxItems: math.MaxInt64,
	})
	all, _, _ := db.Config.LoadItems(app.Filters {
		LoadItemsFilter: app.LoadItemsFilter{
			Deleted: []bool{false},
		},
		MaxItems: math.MaxInt64,
	})

	u := nodeinfo.Usage{
		Users: nodeinfo.UsageUsers{
			Total: len(us),
		},
		LocalComments: len(all)-len(posts),
		LocalPosts: len(posts),
	}
	return u, nil
}


func NodeInfoConfig() nodeinfo.Config {
	return nodeinfo.Config{
		BaseURL: BaseURL,
		InfoURL: "/nodeinfo",

		Metadata: nodeinfo.Metadata{
			NodeName:        app.Instance.NodeInfo().Title,
			NodeDescription: app.Instance.NodeInfo().Summary,
			Private:         false,
			Software: nodeinfo.SoftwareMeta{
				GitHub:   "https://github.com/mariusor/littr.go",
				HomePage: "https://littr.me",
				Follow:   "mariusor@metalhead.club",
			},
		},
		Protocols: []nodeinfo.NodeProtocol{
			nodeinfo.ProtocolActivityPub,
		},
		Services: nodeinfo.Services{
			Inbound:  []nodeinfo.NodeService{},
			Outbound: []nodeinfo.NodeService{},
		},
		Software: nodeinfo.SoftwareInfo{
			Name:    app.Instance.NodeInfo().Title,
			Version: app.Instance.NodeInfo().Version,
		},
	}
}

// HandleHostMeta serves /.well-known/host-meta
func HandleHostMeta(w http.ResponseWriter, r *http.Request) {
	hm := node{
		Links: []link{
			{
				Rel:      "lrdd",
				Type:     "application/xrd+json",
				Template: fmt.Sprintf("%s/.well-known/node?resource={uri}", app.Instance.BaseURL),
			},
		},
	}
	dat, _ := json.Marshal(hm)

	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

// HandleWebFinger serves /.well-known/webfinger/
func (h handler) HandleWebFinger(w http.ResponseWriter, r *http.Request) {
	typ, res := func(res string) (string, string) {
		split := ":"
		if strings.Contains(res, "://") {
			split = "://"
		}
		ar := strings.Split(res, split)
		if len(ar) != 2 {
			return "", ""
		}
		return ar[0], ar[1]
	}(r.URL.Query()["resource"][0])

	if typ == "" || res == "" {
		h.HandleError(w, r, errors.BadRequestf("invalid resource"))
		return
	}

	handle := res
	var addr string
	if typ == "acct" {
		aElem := strings.Split(res, "@")
		if len(aElem) > 1 {
			handle = aElem[0]
			addr = aElem[1]
		}
	} else if typ == "http" || typ == "https" {
		handle = "self"
		addr = res
	}
	if addr != app.Instance.HostName {
		h.HandleError(w, r, errors.NotFoundf("trying to find non-local account"))
		return
	}

	wf := node{}
	val := r.Context().Value(app.RepositoryCtxtKey)
	if handle == "self" {
		var err error
		var inf app.Info
		if repo, ok := val.(app.CanLoadInfo); ok {
			if inf, err = repo.LoadInfo(); err != nil {
				h.HandleError(w, r, errors.NewNotValid(err, "ooops!"))
				return
			}
		}

		id := h.repo.BaseURL + "/self"
		wf.Aliases = []string{
			string(id),
		}
		wf.Subject = inf.URI
		wf.Links = []link{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: string(id),
			},
			{
				Rel:  "service",
				Type: "application/activity+json",
				Href: string(id),
			},
			{
				Rel:  "service",
				Type: "text/html",
				Href: inf.URI,
			},
		}
	} else {
		AcctLoader, ok := val.(app.CanLoadAccounts)
		if !ok {
			err := errors.NotValidf("could not load account repository from Context")
			h.logger.Error(err.Error())
			h.HandleError(w, r, err)
			return
		}
		a, err := AcctLoader.LoadAccount(app.Filters{LoadAccountsFilter:app.LoadAccountsFilter{Handle: []string{handle}}})
		if err != nil {
			err := errors.NotFoundf("resource not found")
			h.logger.Error(err.Error())
			h.HandleError(w, r, err)
			return
		}

		wf.Aliases = []string{
			string(BuildActorID(a)),
			fmt.Sprintf("%s/%s", ActorsURL, a.Handle),
		}
		wf.Subject = typ + ":" + res
		wf.Links = []link{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: string(BuildActorID(a)),
			},
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "application/activity+json",
				Href: string(BuildActorID(a)),
			},
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: accountURL(a).String(),
			},
		}
	}

	dat, _ := json.Marshal(wf)
	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}
