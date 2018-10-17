package frontend

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/mariusor/littr.go/app"

	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

const (
	MaxContentItems = 50
)

func isYay(v *models.Vote) bool {
	return v != nil && v.Weight > 0
}

func isNay(v *models.Vote) bool {
	return v != nil && v.Weight < 0
}

type AccountMetadata struct {
	password string
	salt     string
}

type indexModel struct {
	Title         string
	InvertedTheme bool
	Items         comments
	User          *models.Account
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

func parentLink(c models.Item) string {
	if c.Parent != nil {
		return fmt.Sprintf("/item/%s", c.Parent.Hash)
	}
	return ""
}

func opLink(c models.Item) string {
	if c.OP != nil {
		return fmt.Sprintf("/item/%s", c.OP.Hash)
	}
	return ""
}

func AccountPermaLink(a models.Account) string {
	handle := "anonymous"
	if len(a.Handle) > 0 {
		handle = a.Handle
	}
	return fmt.Sprintf("%s/~%s", app.Instance.BaseURL, handle)
}

func ItemPermaLink(c models.Item) string {
	if c.SubmittedBy == nil {
		return fmt.Sprintf("/item/%s", c.Hash)
	}
	return fmt.Sprintf("%s/%s", AccountPermaLink(*c.SubmittedBy), c.Hash)
}

func scoreLink(i models.Item, dir string) string {
	return fmt.Sprintf("%s/%s", ItemPermaLink(i), dir)
}

func yayLink(i models.Item) string {
	return scoreLink(i, "yay")
}

func nayLink(i models.Item) string {
	return scoreLink(i, "nay")
}

// HandleIndex serves / request
func HandleIndex(w http.ResponseWriter, r *http.Request) {
	m := indexModel{Title: "Index", InvertedTheme: isInverted(r)}

	val := r.Context().Value(models.RepositoryCtxtKey)
	itemLoader, ok := val.(models.CanLoadItems)
	if !ok {
		Logger.WithFields(log.Fields{}).Errorf("could not load item repository from Context")
		return
	}
	items, err := itemLoader.LoadItems(models.LoadItemsFilter{
		Context:  []string{"0"},
		MaxItems: MaxContentItems,
		Deleted:  []bool{false},
	})
	if err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}

	ShowItemData = false
	m.Items = loadComments(items)

	acct, ok := models.ContextCurrentAccount(r.Context())
	if acct.IsLogged() {
		votesLoader, ok := val.(models.CanLoadVotes)
		if ok {
			filters := models.LoadVotesFilter{
				AttributedTo: []models.Hash{acct.Hash},
				ItemKey:      m.Items.getItemsHashes(),
				MaxItems:     MaxContentItems,
			}
			acct.Votes, err = votesLoader.LoadVotes(filters)
			if err != nil {
				Logger.WithFields(log.Fields{}).Error(err)
			}

		} else {
			Logger.WithFields(log.Fields{}).Errorf("could not load vote repository from Context")
		}
	}
	RenderTemplate(r, w, "listing", m)
}

// HandleAbout serves /about request
// It's something Mastodon compatible servers should show
func HandleAbout(w http.ResponseWriter, r *http.Request) {
	ifErr := func(err ...error) {
		if err != nil && len(err) > 0 && err[0] != nil {
			HandleError(w, r, http.StatusInternalServerError, err...)
			return
		}
	}

	m := aboutModel{Title: "About", InvertedTheme: isInverted(r)}
	f, err := os.Open("./README.md")
	ifErr(err)

	st, err := f.Stat()
	ifErr(err)

	data := make([]byte, st.Size())
	io.ReadFull(f, data)
	m.Desc.Description = string(bytes.Trim(data, "\x00"))

	RenderTemplate(r, w, "about", m)
}
