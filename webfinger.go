package brutalinks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"regexp"

	"git.sr.ht/~mariusor/brutalinks/internal/assets"
	log "git.sr.ht/~mariusor/lw"
	"github.com/go-ap/filters"
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
	actorsFilter = filters.HasType(ValidActorTypes...)
	postsFilter  = filters.All(
		filters.HasType(ValidContentTypes...),
		filters.Not(filters.NilInReplyTo),
	)
	allFilter = filters.HasType(ValidContentTypes...)
)

func NodeInfoResolverNew(r *repository) NodeInfoResolver {
	n := NodeInfoResolver{}
	if r == nil {
		return n
	}

	loadFn := func(f filters.Check, fn func(int) error) error {
		res, err := r.b.Search(f)
		if err != nil {
			return err
		}
		return fn(len(res))
	}

	_ = loadFn(actorsFilter, func(cnt int) error {
		n.users = cnt
		return nil
	})
	_ = loadFn(postsFilter, func(cnt int) error {
		n.posts = cnt
		return nil
	})
	_ = loadFn(allFilter, func(cnt int) error {
		n.comments = cnt - n.posts
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
	_, _ = w.Write(dat)
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
