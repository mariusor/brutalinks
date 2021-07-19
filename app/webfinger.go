package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/writeas/go-nodeinfo"
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
		Type: ActivityTypesFilter(ValidContentTypes...),
		OP:   nilIRIs,
	}
	allFilter = &Filters{
		Type: ActivityTypesFilter(ValidContentTypes...),
	}
)

func NodeInfoResolverNew(f *fedbox) NodeInfoResolver {
	n := NodeInfoResolver{}
	if f == nil {
		return n
	}

	us, _ := f.Actors(context.TODO(), Values(actorsFilter))
	if us != nil {
		n.users = int(us.Count())
	}

	posts, _ := f.Objects(context.TODO(), Values(postsFilter))
	if posts != nil {
		n.posts = int(posts.Count())
	}
	all, _ := f.Objects(context.TODO(), Values(allFilter))

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
	dat, _ := json.Marshal(hm)

	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

const selfName = "self"

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
		typ := ar[0]
		handle := ar[1]
		if handle[0] == '@' && len(handle) > 1 {
			handle = handle[1:]
		}
		return typ, handle
	}(res)

	if typ == "" || handle == "" {
		errors.HandleError(errors.BadRequestf("invalid resource %s", res)).ServeHTTP(w, r)
		return
	}

	wf := node{}
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
	var a *Account
	fedbox := h.storage.fedbox.Service()
	handleIRI := pub.IRI(fmt.Sprintf("https://%s/", handle))
	if fedbox.GetLink().Equals(handleIRI, false) || handle == selfName {
		a = new(Account)
		if err := a.FromActivityPub(fedbox); err != nil {
			err := errors.NotFoundf("resource not found %s", res)
			h.errFn()("Error: %s", err)
			errors.HandleError(err).ServeHTTP(w, r)
			return
		}
	} else {
		ff := &Filters{Name: CompStrs{EqualsString(handle)}}
		accounts, _, err := h.storage.LoadAccounts(context.TODO(), nil, ff)
		if err != nil {
			err := errors.NotFoundf("resource not found %s", res)
			h.errFn()("Error: %s", err)
			errors.HandleError(err).ServeHTTP(w, r)
			return
		}
		a, err = accounts.First()
		if err != nil {
			err := errors.NotFoundf("resource not found %s", res)
			h.errFn()("Error: %s", err)
			errors.HandleError(err).ServeHTTP(w, r)
			return
		}
	}
	id := a.GetLink()
	url := accountURL(*a).String()
	url1 := a.Metadata.URL
	wf.Aliases = []string{id, url}
	wf.Subject = res
	wf.Links = []link{
		{
			Rel:  "self",
			Type: "application/activity+json",
			Href: id,
		},
		{
			Rel:  "http://webfinger.net/rel/profile-page",
			Type: "text/html",
			Href: url,
		},
		{
			Rel: "http://ostatus.org/schema/1.0/subscribe",
		},
	}
	if url1 != url && url1 != id {
		wf.Links = append(wf.Links, link{
			Rel:  "http://webfinger.net/rel/profile-page",
			Type: "text/html",
			Href: url1,
		})
	}

	dat, _ := json.Marshal(wf)
	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}
