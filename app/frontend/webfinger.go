package frontend

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/writeas/go-nodeinfo"
	"math"
	"net/http"
	"strings"

	"github.com/mariusor/littr.go/app"

	"github.com/go-ap/errors"
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

type NodeInfoResolver struct{
	storage app.Repository
}

func NodeInfoResolverNew(c Config) NodeInfoResolver {
	return NodeInfoResolver{
		storage: NewRepository(c),
	}
}

func (n NodeInfoResolver) IsOpenRegistration() (bool, error) {
	return false, nil
}

func (n NodeInfoResolver) Usage() (nodeinfo.Usage, error) {
	us, _, _ := n.storage.LoadAccounts(app.Filters{
		LoadAccountsFilter: app.LoadAccountsFilter{
			//IRI:  app.Instance.APIURL,
			Deleted: []bool{false},
		},
		MaxItems: math.MaxInt64,
	})

	posts, _, _ := n.storage.LoadItems(app.Filters{
		LoadItemsFilter: app.LoadItemsFilter{
			Deleted: []bool{false},
			Context: []string{"0"},
		},
		MaxItems: math.MaxInt64,
	})
	all, _, _ := n.storage.LoadItems(app.Filters{
		LoadItemsFilter: app.LoadItemsFilter{
			Deleted: []bool{false},
		},
		MaxItems: math.MaxInt64,
	})

	u := nodeinfo.Usage{
		Users: nodeinfo.UsageUsers{
			Total: len(us),
		},
		LocalComments: len(all) - len(posts),
		LocalPosts:    len(posts),
	}
	return u, nil
}

func NodeInfoConfig() nodeinfo.Config {
	return nodeinfo.Config{
		BaseURL: app.Instance.BaseURL,
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
		//Services: nodeinfo.Services{
		//	Inbound:  []nodeinfo.NodeService{},
		//	Outbound: []nodeinfo.NodeService{},
		//},
		Software: nodeinfo.SoftwareInfo{
			Name:    app.Instance.NodeInfo().Title,
			Version: app.Instance.NodeInfo().Version,
		},
	}
}

// HandleHostMeta serves /.well-known/host-meta
func (h handler) HandleHostMeta(w http.ResponseWriter, r *http.Request) {
	hm := node{
		Subject: "",
		Aliases: nil,
		Links: []link{
			{
				Rel:      "lrdd",
				Type:     "application/xrd+json",
				Template: fmt.Sprintf("%s/.well-known/node?resource={uri}", h.conf.BaseURL),
			},
		},
	}
	dat, _ := xml.Marshal(hm)

	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

// HandleWebFinger serves /.well-known/webfinger/
func (h handler) HandleWebFinger(w http.ResponseWriter, r *http.Request) {
	res := r.URL.Query()["resource"][0]

	typ, handle := func(res string) (string, string) {
		split := ":"
		if strings.Contains(res, "://") {
			split = "://"
		}
		ar := strings.Split(res, split)
		if len(ar) != 2 {
			return "", ""
		}
		return ar[0], ar[1]
	}(res)

	if typ == "" || handle == "" {
		errors.HandleError(errors.BadRequestf("invalid resource %s", res)).ServeHTTP(w, r)
		return
	}

	wf := node{}
	if handle == "self" {
		var err error
		var inf app.Info
		if inf, err = h.storage.LoadInfo(); err != nil {
			errors.HandleError(errors.NewNotValid(err, "ooops!")).ServeHTTP(w, r)
			return
		}

		id := h.storage.BaseURL
		wf.Aliases = []string{
			id,
		}
		wf.Subject = inf.URI
		wf.Links = []link{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: id,
			},
			{
				Rel:  "service",
				Type: "application/activity+json",
				Href: id,
			},
			{
				Rel:  "service",
				Type: "text/html",
				Href: inf.URI,
			},
		}
	} else {
		if strings.Contains(handle, "@") {
			handle, _ = func(s string) (string, string){
				split := "@"
				ar := strings.Split(s, split)
				if len(ar) != 2 {
					return "", ""
				}
				return ar[0], ar[1]
			}(handle)
		}
		a, err := h.storage.LoadAccount(app.Filters{LoadAccountsFilter: app.LoadAccountsFilter{Handle: []string{handle}}})
		if err != nil {
			err := errors.NotFoundf("resource not found %s", res)
			h.logger.Error(err.Error())
			errors.HandleError(err).ServeHTTP(w, r)
			return
		}

		wf.Aliases = []string{
			string(BuildActorID(a)),
			fmt.Sprintf("%s/%s", ActorsURL, a.Handle),
		}
		wf.Subject = res
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
