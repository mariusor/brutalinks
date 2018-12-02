package frontend

import (
	"context"
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/log"
	"github.com/mariusor/qstring"
	"html/template"
	"net/http"
	"os"
	"path"

	"github.com/go-chi/chi"
	"github.com/juju/errors"
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
			"handle": h.account.Handle,
			"hash":   h.account.Hash,
		}).Debug("showing followed posts")
		filter.FollowedBy = []string{h.account.Hash.String()}
	default:
	}
	if m, err := loadItems(r.Context(), filter, &h.account, h.logger); err == nil {
		m.Title = "Index"

		m.HideText = true
		if len(m.Items) >= MaxContentItems {
			m.NextPage = filter.Page + 1
		}
		if filter.Page > 1 {
			m.PrevPage = filter.Page - 1
		}
		h.RenderTemplate(r, w, "listing", m)
	} else {
		h.HandleError(w, r, errors.NewNotValid(err, "oops!"))
	}
}

func loadItems(c context.Context, filter app.LoadItemsFilter, acc *app.Account, l log.Logger) (itemListingModel, error) {
	m := itemListingModel{}

	itemLoader, ok := app.ContextItemLoader(c)
	if !ok {
		err := errors.Errorf("could not load item repository from Context")
		return m, err
	}
	contentItems, err := itemLoader.LoadItems(filter)
	if err != nil {
		return m, err
	}
	m.Items = loadComments(contentItems)
	replaceTags(m.Items)
	if acc.IsLogged() {
		votesLoader, ok := app.ContextVoteLoader(c)
		if ok {
			acc.Votes, err = votesLoader.LoadVotes(app.LoadVotesFilter{
				AttributedTo: []app.Hash{acc.Hash},
				ItemKey:      m.Items.getItemsHashes(),
				MaxItems:     MaxContentItems,
			})
			if err != nil {
				l.Error(err.Error())
			}
		} else {
			l.Error("could not load vote repository from Context")
		}
	}
	return m, nil
}

// HandleDomains serves /tags/{domain} request
func (h *handler) HandleTags(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	filter := app.LoadItemsFilter{
		Content:          fmt.Sprintf("#%s", tag),
		ContentMatchType: app.MatchFuzzy,
		MaxItems:         MaxContentItems,
		Page:             1,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}
	if m, err := loadItems(r.Context(), filter, &h.account, h.logger); err == nil {
		m.Title = fmt.Sprintf("Submissions tagged as #%s", tag)

		if len(m.Items) >= MaxContentItems {
			m.NextPage = filter.Page + 1
		}
		if filter.Page > 1 {
			m.PrevPage = filter.Page - 1
		}
		h.RenderTemplate(r, w, "listing", m)
	} else {
		h.HandleError(w, r, errors.NewNotValid(err, "oops!"))
	}
}

// HandleDomains serves /domains/{domain} request
func (h *handler) HandleDomains(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	filter := app.LoadItemsFilter{
		Content:          fmt.Sprintf("http[s]?://%s", domain),
		ContentMatchType: app.MatchFuzzy,
		MediaType:        []string{app.MimeTypeURL},
		MaxItems:         MaxContentItems,
		Page:             1,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}
	if m, err := loadItems(r.Context(), filter, &h.account, h.logger); err == nil {
		m.Title = fmt.Sprintf("Submissions from %s", domain)

		m.HideText = true
		if len(m.Items) >= MaxContentItems {
			m.NextPage = filter.Page + 1
		}
		if filter.Page > 1 {
			m.PrevPage = filter.Page - 1
		}
		h.RenderTemplate(r, w, "listing", m)
	} else {
		h.HandleError(w, r, errors.NewNotValid(err, "oops!"))
	}
}
