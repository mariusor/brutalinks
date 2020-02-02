package app

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/go-ap/activitypub"
	"github.com/writeas/go-nodeinfo"
	"net/http"
	"strings"

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

type NodeInfoResolver struct {
	storage  *repository
	users    int
	comments int
	posts    int
}

func NodeInfoResolverNew(stor *repository) NodeInfoResolver {
	n := NodeInfoResolver{}
	fp := fedFilters{
		Type: ActivityTypesFilter(ValidActorTypes...),
	}
	if item, err := stor.fedbox.Actors(Values(&fp)); err != nil {
		stor.errFn(fmt.Sprintf("unable to load actors: %s", err), nil)
	} else {
		activitypub.OnOrderedCollection(item, func(c *activitypub.OrderedCollection) error {
			n.users = int(c.TotalItems)
			return nil
		})
	}
	fi := fedFilters{
		Type: ActivityTypesFilter(ValidItemTypes...),
		OP: nilIRIs,
	}
	if item, err := stor.fedbox.Objects(Values(&fi)); err != nil {
		stor.errFn(fmt.Sprintf("unable to load items: %s", err), nil)
	} else {
		activitypub.OnOrderedCollection(item, func(c *activitypub.OrderedCollection) error {
			n.posts = int(c.TotalItems)
			return nil
		})
	}
	fa := fedFilters{
		Type: ActivityTypesFilter(ValidItemTypes...),
	}
	if item, err := stor.fedbox.Objects(Values(&fa)); err != nil {
		stor.errFn(fmt.Sprintf("unable to load all coments: %s", err), nil)
	} else {
		activitypub.OnOrderedCollection(item, func(c *activitypub.OrderedCollection) error {
			n.comments = int(c.TotalItems) - n.posts
			return nil
		})
	}
	return n
}

func (n NodeInfoResolver) IsOpenRegistration() (bool, error) {
	return Instance.Config.UserCreatingEnabled, nil
}

func (n NodeInfoResolver) Usage() (nodeinfo.Usage, error) {
	u := nodeinfo.Usage{
		Users: nodeinfo.UsageUsers{
			Total: n.users,
		},
		LocalComments: n.comments,
		LocalPosts:    n.posts,
	}
	return u, nil
}

func NodeInfoConfig() nodeinfo.Config {
	return nodeinfo.Config{
		BaseURL: Instance.BaseURL,
		InfoURL: "/nodeinfo",

		Metadata: nodeinfo.Metadata{
			NodeName:        Instance.NodeInfo().Title,
			NodeDescription: Instance.NodeInfo().Summary,
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
			Name:    "littr",
			Version: Instance.NodeInfo().Version,
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
		var inf WebInfo
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
			handle, _ = func(s string) (string, string) {
				split := "@"
				ar := strings.Split(s, split)
				if len(ar) != 2 {
					return "", ""
				}
				return ar[0], ar[1]
			}(handle)
		}
		a, err := h.storage.LoadAccount(Filters{LoadAccountsFilter: LoadAccountsFilter{Handle: []string{handle}}})
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
