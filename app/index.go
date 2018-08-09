package app

import (
	"bytes"
	"fmt"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
			)

const (
	MaxContentItems = 200
)

func IsYay(v *models.Vote)  bool {
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
	if i.Path == nil {
		return 0
	}
	return bytes.Count(i.Path, []byte(".")) + 1
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
	if len(c.Path) == 0 {
		return "/"
	} else {
		lastDotPos := bytes.LastIndex(c.Path, []byte(".")) + 1
		parentHash := c.Path[lastDotPos : lastDotPos+8]
		return fmt.Sprintf("/parent/%s/%s", c.Hash, parentHash)
	}
}
func OPLink(c models.Item) string {
	if len(c.Path) > 0 {
		parentHash := c.Path[0:8]
		return fmt.Sprintf("/op/%s/%s", c.Hash, parentHash)
	}
	return "/"
}

func permaLink(c models.Item) string {
	if c.SubmittedBy == nil {
		return ""
	}
	return fmt.Sprintf("/~%s/%s", c.SubmittedBy.Handle, c.Hash)
}

func scoreLink(i models.Item, dir string) string {
	return fmt.Sprintf("%s/%s", permaLink(i), dir)
}
func YayLink(i models.Item) string {
	return scoreLink(i, "yay")
}
func NayLink(i models.Item)  string {
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
		log.Errorf("could not load loader service from Context")
		return
	}
	var err error
	items, err := itemLoader.LoadItems(models.LoadItemsFilter{
		Type: []models.ItemType{models.TypeOP},
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
		CurrentAccount.Votes, err = models.Service.LoadVotes(models.LoadVotesFilter{
			SubmittedBy: []string{CurrentAccount.Hash,},
			ItemKey:     m.Items.getItemsHashes(),
			MaxItems:    MaxContentItems,
		})

		if err != nil {
			log.Error(err)
			//HandleError(w, r, http.StatusNotFound, err)
			//return
		}
	}
	RenderTemplate(r, w, "listing", m)
}
