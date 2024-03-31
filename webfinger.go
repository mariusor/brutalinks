package brutalinks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"regexp"

	"git.sr.ht/~mariusor/brutalinks/internal/assets"
	log "git.sr.ht/~mariusor/lw"
	vocab "github.com/go-ap/activitypub"
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
		OP:   nilFilters,
	}
	allFilter = &Filters{
		Type: ActivityTypesFilter(ValidContentTypes...),
	}
)

func NodeInfoResolverNew(r *repository) NodeInfoResolver {
	n := NodeInfoResolver{}
	if r == nil {
		return n
	}

	base := baseIRI(r.fedbox.Service().GetLink())
	loadFn := func(f *Filters, fn vocab.WithOrderedCollectionFn) error {
		ff := []*Filters{{Type: CreateActivitiesFilter, Object: f}}
		return LoadFromSearches(context.TODO(), r, RemoteLoads{base: {{loadFn: inbox, filters: ff}}}, func(ctx context.Context, c vocab.CollectionInterface, f *Filters) error {
			return vocab.OnOrderedCollection(c, fn)
		})
	}

	loadFn(actorsFilter, func(col *vocab.OrderedCollection) error {
		n.users = int(col.TotalItems)
		return nil
	})
	loadFn(postsFilter, func(col *vocab.OrderedCollection) error {
		n.posts = int(col.TotalItems)
		return nil
	})
	loadFn(allFilter, func(col *vocab.OrderedCollection) error {
		n.comments = int(col.TotalItems) - n.posts
		return nil
	})
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
	softwareName = "brutalinks"
	sourceURL    = "https://git.sr.ht/~mariusor/brutalinks"
	author       = "@mariusor@metalhead.club"
)

func NodeInfoConfig() nodeinfo.Config {
	ni := Instance.NodeInfo()
	return nodeinfo.Config{
		BaseURL: Instance.BaseURL.String(),
		InfoURL: "/nodeinfo",

		Metadata: nodeinfo.Metadata{
			NodeName:        string(regexp.MustCompile(`<[\/\w]+>`).ReplaceAll([]byte(ni.Title), []byte{})),
			NodeDescription: ni.Summary,
			Private:         !Instance.Conf.UserCreatingEnabled,
			Software: nodeinfo.SoftwareMeta{
				GitHub:   sourceURL,
				HomePage: Instance.BaseURL.String(),
				Follow:   Instance.Conf.AdminContact,
			},
		},
		Protocols: []nodeinfo.NodeProtocol{
			nodeinfo.ProtocolActivityPub,
		},
		Services: nodeinfo.Services{
			Inbound:  []nodeinfo.NodeService{},
			Outbound: []nodeinfo.NodeService{nodeinfo.ServiceAtom, nodeinfo.ServiceRSS},
		},
		Software: nodeinfo.SoftwareInfo{
			Name:    path.Base(softwareName),
			Version: ni.Version,
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

func (a Application) NodeInfo() WebInfo {
	// Name formats the name of the current Application
	inf := WebInfo{
		Title:   a.Conf.Name,
		Summary: "Link aggregator inspired by reddit and hacker news using ActivityPub federation.",
		Email:   a.Conf.AdminContact,
		URI:     a.BaseURL.String(),
		Version: a.Version,
	}

	if desc, err := fs.ReadFile(assets.AssetFS, "README.md"); err == nil {
		inf.Description = string(bytes.Trim(desc, "\x00"))
	} else {
		a.Logger.WithContext(log.Ctx{"err": err}).Errorf("unable to load README.md file from fs: %s", assets.AssetFS)
	}
	return inf
}
