package frontend

import (
	"fmt"
	"net/http"
	"os"

	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

const (
	MaxContentItems = 200
)

func IsYay(v *models.Vote) bool {
	return v != nil && v.Weight > 0
}

func IsNay(v *models.Vote) bool {
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

func ParentLink(c models.Item) string {
	if c.Parent != nil {
		return fmt.Sprintf("/item/%s", c.Parent.Hash)
	}
	return ""
}

func OPLink(c models.Item) string {
	if c.OP != nil {
		return fmt.Sprintf("/item/%s", c.OP.Hash)
	}
	return ""
}

func permaLink(c models.Item) string {
	handle := "anonymous"
	if c.SubmittedBy != nil {
		handle = c.SubmittedBy.Handle
	}
	return fmt.Sprintf("/~%s/%s", handle, c.Hash)
}

func scoreLink(i models.Item, dir string) string {
	return fmt.Sprintf("%s/%s", permaLink(i), dir)
}
func YayLink(i models.Item) string {
	return scoreLink(i, "yay")
}
func NayLink(i models.Item) string {
	return scoreLink(i, "nay")
}

// HandleIndex serves / request
func HandleIndex(w http.ResponseWriter, r *http.Request) {
	m := indexModel{Title: "Index", InvertedTheme: isInverted(r)}

	val := r.Context().Value(RepositoryCtxtKey)
	itemLoader, ok := val.(models.CanLoadItems)
	if ok {
		log.Infof("loaded repository of type %T", itemLoader)
	} else {
		log.WithFields(log.Fields{}).Errorf("could not load item repository from Context")
		return
	}
	items, err := itemLoader.LoadItems(models.LoadItemsFilter{
		Context:  []string{"0"},
		MaxItems: MaxContentItems,
	})
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}

	ShowItemData = false
	m.Items = loadComments(items)

	if CurrentAccount.IsLogged() {
		votesLoader, ok := val.(models.CanLoadVotes)
		if ok {
			log.Infof("loaded repository of type %T", itemLoader)
			filters := models.LoadVotesFilter{
				AttributedTo: []models.Hash{CurrentAccount.Hash},
				ItemKey:      m.Items.getItemsHashes(),
				MaxItems:     MaxContentItems,
			}
			CurrentAccount.Votes, err = votesLoader.LoadVotes(filters)
			if err != nil {
				log.WithFields(log.Fields{}).Error(err)
			}

		} else {
			log.WithFields(log.Fields{}).Errorf("could not load vote repository from Context")
		}
	}
	RenderTemplate(r, w, "listing", m)
}
