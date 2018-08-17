package app

import (
	"fmt"
	"net/http"
	"os"

	"github.com/mariusor/littr.go/models"
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

func (i comment) Level() int {
	//if i.Path == nil {
	//	return 0
	//}
	//return bytes.Count(i.Path, []byte(".")) + 1
	return 0
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

	val := r.Context().Value(ServiceCtxtKey)
	itemLoader, ok := val.(models.CanLoadItems)
	if ok {
		log.Infof("loaded LoaderService of type %T", itemLoader)
	} else {
		log.Errorf("could not load item loader service from Context")
		return
	}
	var err error
	items, err := itemLoader.LoadItems(models.LoadItemsFilter{
		Context:  []string{"0"},
		MaxItems: MaxContentItems,
	})
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}

	ShowItemData = false
	m.Items = loadComments(items)

	if CurrentAccount.IsLogged() {
		votesLoader, ok := val.(models.CanLoadVotes)
		if ok {
			log.Infof("loaded LoaderService of type %T", itemLoader)
			CurrentAccount.Votes, err = votesLoader.LoadVotes(models.LoadVotesFilter{
				SubmittedBy: []string{CurrentAccount.Hash},
				ItemKey:     m.Items.getItemsHashes(),
				MaxItems:    MaxContentItems,
			})

			if err != nil {
				log.Error(err)
			}
		} else {
			log.Errorf("could not load vote loader service from Context")
		}
	}
	RenderTemplate(r, w, "listing", m)
}
