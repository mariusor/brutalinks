package frontend

import (
	"context"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/mariusor/qstring"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
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
	Title string
	Desc  app.Desc
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
		// @todo(marius) :link_generation:
		return fmt.Sprintf("/i/%s", c.Parent.Hash)
	}
	return ""
}

func opLink(c app.Item) string {
	if c.OP != nil {
		// @todo(marius) :link_generation:
		return fmt.Sprintf("/i/%s", c.OP.Hash)
	}
	return ""
}

func AccountLocalLink(a app.Account) string {
	handle := "anonymous"
	if len(a.Handle) > 0 {
		handle = a.Handle
	}
	// @todo(marius) :link_generation:
	return fmt.Sprintf("/~%s", handle)
}

// ShowAccountHandle
func ShowAccountHandle(a app.Account) string {
	//if strings.Contains(a.Handle, "@") {
	//	// @TODO(marius): simplify this at a higher level in the stack, see Account::FromActivityPub
	//	if parts := strings.SplitAfter(a.Handle, "@"); len(parts) > 1 {
	//		if strings.Contains(parts[1], app.Instance.HostName) {
	//			handle := parts[0]
	//			a.Handle = handle[:len(handle)-1]
	//		}
	//	}
	//}
	return a.Handle
}

// AccountPermaLink
func AccountPermaLink(a app.Account) string {
	if a.HasMetadata() && len(a.Metadata.URL) > 0 {
		return a.Metadata.URL
	}
	return AccountLocalLink(a)
}

// ItemPermaLink
func ItemPermaLink(i app.Item) string {
	if !i.IsLink() && i.HasMetadata() && len(i.Metadata.URL) > 0 {
		return i.Metadata.URL
	}
	return ItemLocalLink(i)
}

// ItemLocalLink
func ItemLocalLink(i app.Item) string {
	if i.SubmittedBy == nil {
		// @todo(marius) :link_generation:
		return fmt.Sprintf("/i/%s", i.Hash.Short())
	}
	return fmt.Sprintf("%s/%s", AccountLocalLink(*i.SubmittedBy), i.Hash.Short())
}

func scoreLink(i app.Item, dir string) string {
	// @todo(marius) :link_generation:
	return fmt.Sprintf("%s/%s", ItemPermaLink(i), dir)
}

func yayLink(i app.Item) string {
	return scoreLink(i, "yay")
}

func nayLink(i app.Item) string {
	return scoreLink(i, "nay")
}

func canPaginate(m interface{}) bool {
	_, ok := m.(Paginator)
	return ok
}

func pageLink(p int) template.HTML {
	if p >= 1 {
		return template.HTML(fmt.Sprintf("?page=%d", p))
	} else {
		return template.HTML("")
	}
}

// HandleIndex serves / request
func (h *handler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	filter := app.Filters{
		LoadItemsFilter: app.LoadItemsFilter{
			InReplyTo: []string{""},
			Deleted:   []bool{false},
			Federated: []bool{false},
			Private:   []bool{false},
		},
		Page:     1,
		MaxItems: MaxContentItems,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}

	baseURL, _ := url.Parse(h.conf.BaseURL)
	title := fmt.Sprintf("%s: main page", baseURL.Host)

	acct := h.account(r)
	base := path.Base(r.URL.Path)
	switch strings.ToLower(base) {
	case "self":
		title = fmt.Sprintf("%s: self", baseURL.Host)
		h.logger.Debug("showing self posts")
	case "federated":
		title = fmt.Sprintf("%s: federated", baseURL.Host)
		h.logger.Debug("showing federated posts")
		filter.Federated = []bool{true}
	default:
	}
	m := itemListingModel{}
	m.Title = title
	m.HideText = true
	comments, err := loadItems(r.Context(), filter, acct, h.logger)
	if err != nil {
		h.HandleErrors(w, r, errors.NewNotValid(err, "Unable to load items!"))
	}
	for _, c := range comments {
		m.Items = append(m.Items, c)
	}
	if len(comments) >= filter.MaxItems {
		m.nextPage = filter.Page + 1
	}
	if filter.Page > 1 {
		m.prevPage = filter.Page - 1
	}
	h.RenderTemplate(r, w, "listing", m)
}

type follow struct {
	app.FollowRequest
}

func (f *follow) Type() RenderType {
	return FollowRequest
}

// HandleIndex serves / request
func (h *handler) HandleInbox(w http.ResponseWriter, r *http.Request) {
	filter := app.Filters{
		LoadItemsFilter: app.LoadItemsFilter{
			InReplyTo: []string{""},
			Deleted:   []bool{false},
			Federated: []bool{false},
			Private:   []bool{false},
		},
		Page:     1,
		MaxItems: MaxContentItems,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}

	baseURL, _ := url.Parse(h.conf.BaseURL)
	title := fmt.Sprintf("%s: main page", baseURL.Host)

	acct := h.account(r)
	title = fmt.Sprintf("%s: followed", baseURL.Host)

	filter.FollowedBy = acct.Hash.String()

	m := itemListingModel{}
	m.Title = title
	m.HideText = true

	curActor := loadAPPerson(*acct)
	followReq, err := h.storage.fedbox.Inbox(curActor, func() url.Values {
		return url.Values{
			"type": {
				string(pub.FollowType),
			},
		}
	})
	if err == nil && len(followReq.Collection()) > 0 {
		requests := make([]app.FollowRequest, 0)
		for _, fr := range followReq.Collection() {
			f := app.FollowRequest{}
			if err := f.FromActivityPub(fr); err == nil {
				if !accountInCollection(*f.SubmittedBy, acct.Followers) {
					requests = append(requests, f)
				}
			}
		}
		requests, err = h.storage.loadAuthors(requests...)
		for _, r := range requests {
			f := follow{r}
			m.Items = append(m.Items, &f)
		}
	}
	comments, err := loadItems(r.Context(), filter, acct, h.logger)
	if err != nil {
		h.HandleErrors(w, r, errors.NewNotValid(err, "Unable to load items!"))
	}
	for _, c := range comments {
		m.Items = append(m.Items, c)
	}
	if len(comments) >= filter.MaxItems {
		m.nextPage = filter.Page + 1
	}
	if filter.Page > 1 {
		m.prevPage = filter.Page - 1
	}

	h.RenderTemplate(r, w, "listing", m)
}
func loadItems(c context.Context, filter app.Filters, acc *app.Account, l log.Logger) (comments, error) {
	itemLoader, ok := app.ContextItemLoader(c)
	if !ok {
		err := errors.Errorf("could not load item repository from Context")
		return nil, err
	}
	contentItems, _, err := itemLoader.LoadItems(filter)

	if err != nil {
		return nil, err
	}
	comments := loadComments(contentItems)
	if acc.IsLogged() {
		votesLoader, ok := app.ContextVoteLoader(c)
		if ok {
			acc.Votes, _, err = votesLoader.LoadVotes(app.Filters{
				LoadVotesFilter: app.LoadVotesFilter{
					AttributedTo: []app.Hash{acc.Hash},
					ItemKey:      comments.getItemsHashes(),
				},
				MaxItems: MaxContentItems,
			})
			if err != nil {
				l.Error(err.Error())
			}
		} else {
			l.Error("could not load vote repository from Context")
		}
	}
	return comments, nil
}

// HandleTags serves /tags/{tag} request
func (h *handler) HandleTags(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	filter := app.Filters{
		MaxItems: MaxContentItems,
		Page:     1,
	}
	acct := h.account(r)
	if len(tag) == 0 {
		h.HandleErrors(w, r, errors.BadRequestf("missing tag"))
	}
	filter.Content = "#" + tag
	filter.ContentMatchType = app.MatchFuzzy
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}
	baseURL, _ := url.Parse(h.conf.BaseURL)
	m := itemListingModel{}
	m.Title = fmt.Sprintf("%s: tagged as #%s", baseURL.Host, tag)
	comments, err := loadItems(r.Context(), filter, acct, h.logger)
	if err != nil {
		h.HandleErrors(w, r, errors.NewNotValid(err, "oops!"))
	}
	for _, c := range comments {
		m.Items = append(m.Items, c)
	}
	if len(comments) >= filter.MaxItems {
		m.nextPage = filter.Page + 1
	}
	if filter.Page > 1 {
		m.prevPage = filter.Page - 1
	}
	h.RenderTemplate(r, w, "listing", m)
}

// HandleDomains serves /domains/{domain} request
func (h *handler) HandleDomains(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")

	acct := h.account(r)
	filter := app.Filters{
		LoadItemsFilter: app.LoadItemsFilter{
			Context: []string{"0"},
		},
		MaxItems: MaxContentItems,
		Page:     1,
	}
	if len(domain) > 0 {
		filter.LoadItemsFilter.URL = domain
		filter.Type = pub.ActivityVocabularyTypes{pub.PageType}
	} else {
		filter.MediaType = []app.MimeType{app.MimeTypeMarkdown, app.MimeTypeText, app.MimeTypeHTML}
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filter); err != nil {
		h.logger.Debug("unable to load url parameters")
	}
	baseURL, _ := url.Parse(h.conf.BaseURL)
	m := itemListingModel{}
	m.Title = fmt.Sprintf("%s: from %s", baseURL.Host, domain)
	m.HideText = true
	comments, err := loadItems(r.Context(), filter, acct, h.logger)
	if err != nil {
		h.HandleErrors(w, r, errors.NewNotValid(err, "oops!"))
	}
	for _, c := range comments {
		m.Items = append(m.Items, c)
	}
	if len(comments) >= filter.MaxItems {
		m.nextPage = filter.Page + 1
	}
	if filter.Page > 1 {
		m.prevPage = filter.Page - 1
	}
	h.RenderTemplate(r, w, "listing", m)
}
