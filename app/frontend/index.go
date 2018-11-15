package frontend

import (
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/log"
	"github.com/mariusor/qstring"
	"html/template"
	"net/http"
	"os"
	"path"
)

const (
	MaxContentItems = 50
)

func isYay(v *app.Vote) bool {
	return v != nil && v.Weight > 0
}

func isNay(v *app.Vote) bool {
	return v != nil && v.Weight < 0
}

type AccountMetadata struct {
	password string
	salt     string
}

type aboutModel struct {
	Title         string
	InvertedTheme bool
	Desc          app.Desc
}

func getAuthProviders() map[string]string {
	p := make(map[string]string)
	if os.Getenv("GITHUB_KEY") != "" {
		p["github"] = "Github"
	}
	if os.Getenv("GITLAB_KEY") != "" {
		p["gitlab"] = "Gitlab"
	}
	if os.Getenv("GOOGLE_KEY") != "" {
		p["google"] = "Google"
	}
	if os.Getenv("FACEBOOK_KEY") != "" {
		p["facebook"] = "Facebook"
	}

	return p
}

func parentLink(c app.Item) string {
	if c.Parent != nil {
		return fmt.Sprintf("/item/%s", c.Parent.Hash)
	}
	return ""
}

func opLink(c app.Item) string {
	if c.OP != nil {
		return fmt.Sprintf("/item/%s", c.OP.Hash)
	}
	return ""
}

func AccountPermaLink(a app.Account) string {
	handle := "anonymous"
	if len(a.Handle) > 0 {
		handle = a.Handle
	}
	return fmt.Sprintf("%s/~%s", app.Instance.BaseURL, handle)
}

func ItemPermaLink(c app.Item) string {
	if c.SubmittedBy == nil {
		return fmt.Sprintf("/item/%s", c.Hash)
	}
	return fmt.Sprintf("%s/%s", AccountPermaLink(*c.SubmittedBy), c.Hash)
}

func scoreLink(i app.Item, dir string) string {
	return fmt.Sprintf("%s/%s", ItemPermaLink(i), dir)
}

func yayLink(i app.Item) string {
	return scoreLink(i, "yay")
}

func nayLink(i app.Item) string {
	return scoreLink(i, "nay")
}

func pageLink(p int) template.HTML {
	//if p > 1 {
	return template.HTML(fmt.Sprintf("?page=%d", p))
	//} else {
	//	return template.HTML("")
	//}
}

// HandleIndex serves / request
func (h *handler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	filter := app.LoadItemsFilter{
		Context:  []string{"0"},
		MaxItems: MaxContentItems,
		Deleted:  []bool{false},
		Page:     1,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}

	base := path.Base(r.URL.Path)
	switch base {
	case "self":
		h.logger.Debug("showing self posts")
	case "federated":
		h.logger.Debug("showing federated posts")
		filter.Federated = []bool{true}
	case "followed":
		h.logger.WithContext(log.Ctx{
			"handler": h.account.Handle,
			"hash":    h.account.Hash,
		}).Debug("showing followed posts")
		filter.FollowedBy = []string{h.account.Hash.String()}
	default:
	}
	if m, err := loadItems(r.Context(), filter, &h.account, h.logger); err == nil {
		m.Title = "Index"
		m.InvertedTheme = isInverted(r)

		h.showItemData = false
		if len(m.Items) >= MaxContentItems {
			m.NextPage = filter.Page + 1
		}
		if filter.Page > 1 {
			m.PrevPage = filter.Page - 1
		}
		h.RenderTemplate(r, w, "listing", m)
	} else {
		h.HandleError(w, r, http.StatusInternalServerError, err)
	}
}

// HandleAbout serves /about request
// It's something Mastodon compatible servers should show
func (h *handler) HandleAbout(w http.ResponseWriter, r *http.Request) {
	m := aboutModel{Title: "About", InvertedTheme: isInverted(r)}
	f, err := db.Config.LoadInfo()
	if err != nil {
		h.HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	m.Desc.Description = f.Description

	h.RenderTemplate(r, w, "about", m)
}
