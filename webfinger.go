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
	"strings"

	log "git.sr.ht/~mariusor/lw"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/mariusor/go-littr/internal/assets"
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

func splitResourceString(res string) (string, string) {
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
}

// HandleWebFinger serves /.well-known/webfinger/
func (h handler) HandleWebFinger(w http.ResponseWriter, r *http.Request) {
	res := r.URL.Query().Get("resource")

	typ, handle := splitResourceString(res)
	if typ == "" || handle == "" {
		errors.HandleError(errors.BadRequestf("invalid resource %s", res)).ServeHTTP(w, r)
		return
	}
	var host string

	wf := node{}
	if strings.Contains(handle, "@") {
		handle, host = func(s string) (string, string) {
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
	handleIRI := vocab.IRI(fmt.Sprintf("https://%s/", handle))
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
			err = errors.NewNotFound(err, "resource not found %s", res)
			h.errFn(log.Ctx{"err": err})("unable to load %s", handle)
			errors.HandleError(err).ServeHTTP(w, r)
			return
		}
		a, err = accounts.First()
		if err != nil {
			err = errors.NewNotFound(err, "resource not found %s", res)
			h.errFn()("Error: %s", err)
			errors.HandleError(err).ServeHTTP(w, r)
			return
		}
	}

	id := a.AP().GetID()
	if host == "" {
		host = h.conf.HostName
	}
	wf.Subject = fmt.Sprintf("%s@%s", handle, host)
	wf.Links = []link{
		{
			Rel:  "self",
			Type: "application/activity+json",
			Href: id.String(),
		},
	}
	existsOnInstance := false
	vocab.OnActor(a.AP(), func(act *vocab.Actor) error {
		urls := make(vocab.ItemCollection, 0)
		if vocab.IsItemCollection(act.URL) {
			urls = append(urls, act.URL.(vocab.ItemCollection)...)
		} else {
			urls = append(urls, act.URL.(vocab.IRI))
		}

		for _, u := range urls {
			url := u.GetLink().String()
			existsOnInstance = existsOnInstance || strings.Contains(url, Instance.BaseURL.String())
			wf.Aliases = append(wf.Aliases, url)
			wf.Links = append(wf.Links, link{
				Rel:  "https://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: url,
			})
		}
		return nil
	})
	if !existsOnInstance {
		err := errors.NotFoundf("resource not found %s", res)
		h.errFn()("Error: %s", err)
		errors.HandleError(err).ServeHTTP(w, r)
		return
	}
	wf.Aliases = append(wf.Aliases, id.String())

	dat, _ := json.Marshal(wf)
	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

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
