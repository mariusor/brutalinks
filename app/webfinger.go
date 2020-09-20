package app

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/writeas/go-nodeinfo"
	"net/http"
	"regexp"
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
	users    int
	comments int
	posts    int
}

var (
	actorsFilter = &Filters{
		Type: ActivityTypesFilter(ValidActorTypes...),
	}
	postsFilter = &Filters{
		Type: ActivityTypesFilter(ValidItemTypes...),
		OP:   nilIRIs,
	}
	allFilter = &Filters{
		Type: ActivityTypesFilter(ValidItemTypes...),
	}
)

func NodeInfoResolverNew(f *fedbox) NodeInfoResolver {
	n := NodeInfoResolver{}

	us, _ := f.Actors(context.Background(), Values(actorsFilter))
	if us != nil {
		n.users = int(us.Count())
	}

	posts, _ := f.Objects(context.Background(), Values(postsFilter))
	if posts != nil {
		n.posts = int(posts.Count())
	}
	all, _ := f.Objects(context.Background(), Values(allFilter))

	if all != nil {
		n.comments = int(all.Count()) - n.posts
	}
	return n
}

func (n NodeInfoResolver) IsOpenRegistration() (bool, error) {
	return Instance.Conf.UserCreatingEnabled, nil
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

const (
	githubUrl    = "https://github.com/mariusor/go-littr"
	author       = "@mariusor@metalhead.club"
	softwareName = "go-littr"
)

func NodeInfoConfig() nodeinfo.Config {
	return nodeinfo.Config{
		BaseURL: Instance.BaseURL,
		InfoURL: "/nodeinfo",

		Metadata: nodeinfo.Metadata{
			NodeName:        string(regexp.MustCompile(`<[\/\w]+>`).ReplaceAll([]byte(Instance.NodeInfo().Title), []byte{})),
			NodeDescription: Instance.NodeInfo().Summary,
			Private:         false,
			Software: nodeinfo.SoftwareMeta{
				GitHub:   githubUrl,
				HomePage: Instance.BaseURL,
				Follow:   author,
			},
		},
		Protocols: []nodeinfo.NodeProtocol{
			nodeinfo.ProtocolActivityPub,
		},
		Services: nodeinfo.Services{
			Inbound:  []nodeinfo.NodeService{},
			Outbound: []nodeinfo.NodeService{nodeinfo.ServiceAtom},
		},
		Software: nodeinfo.SoftwareInfo{
			Name:    softwareName,
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
	res := r.URL.Query().Get("resource")

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
		ff := &Filters{Name: CompStrs{EqualsString(handle)}}
		accounts, _, err := h.storage.LoadAccounts(context.Background(), ff)
		if err != nil {
			err := errors.NotFoundf("resource not found %s", res)
			h.errFn()("Error: %s", err)
			errors.HandleError(err).ServeHTTP(w, r)
			return
		}
		a, err := accounts.First()
		if err != nil {
			err := errors.NotFoundf("resource not found %s", res)
			h.errFn()("Error: %s", err)
			errors.HandleError(err).ServeHTTP(w, r)
			return
		}
		wf.Aliases = []string{
			string(BuildActorID(*a)),
			fmt.Sprintf("%s/%s", ActorsURL, a.Handle),
		}
		wf.Subject = res
		wf.Links = []link{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: string(BuildActorID(*a)),
			},
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "application/activity+json",
				Href: string(BuildActorID(*a)),
			},
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: accountURL(*a).String(),
			},
		}
	}

	dat, _ := json.Marshal(wf)
	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}
