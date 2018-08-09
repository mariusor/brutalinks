package app

import (
	"bytes"
	"fmt"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"time"
	)

const (
	MaxContentItems = 200
)

type Account struct {
	Id        int64
	Hash      string    `json:"key"`
	Email     []byte    `json:"email"`
	Handle    string    `json:"handle"`
	Score     int64     `json:"score"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	flags     int8
	metadata  []byte
	votes     map[string]Vote
}

type Vote struct {
	SubmittedBy string    `json:"submitted_by"`
	SubmittedAt time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Weight      int       `json:"weight"`
	ItemHash    string    `json:"item_hash"`
	id          int64
	itemId      int64
	flags       int8
}

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

func LoadVotes(a *models.Account, it []models.Item) ([]Vote, error) {
	return nil, nil
//	var ids = make([]int64, len(it))
//	var hashes = make(map[string]string, 0)
//	for i, k := range it {
//		ids[i] = int64(i)
//		hashes[k.Hash] = k.Hash
//	}
//	if a == nil {
//		return nil, errors.Errorf("no account to load for")
//	}
//	if len(ids) == 0 {
//		log.Error(errors.Errorf("no ids to load"))
//	}
//	// this here code following is the ugliest I wrote in quite a long time
//	// so ugly it warrants its own fucking shame corner
//	sids := make([]string, 0)
//	for i := 0; i < len(ids); i++ {
//		sids = append(sids, fmt.Sprintf("$%d", i+2))
//	}
//	iitems := make([]interface{}, len(ids)+1)
//	iitems[0] = a.Id
//	for i, v := range ids {
//		iitems[i+1] = v
//	}
//	sel := fmt.Sprintf(`select "id", "submitted_by", "submitted_at", "updated_at", "item_id", "weight", "flags"
//	from "votes" where "submitted_by" = $1 and "item_id" in (%s)`, strings.Join(sids, ", "))
//	rows, err := Db.Query(sel, iitems...)
//	if err != nil {
//		return nil, err
//	}
//	if a.votes == nil {
//		a.votes = make(map[string]Vote, 0)
//	}
//RowLoop:
//	for rows.Next() {
//		v := models.Vote{}
//		err = rows.Scan(&v.Id, &v.SubmittedBy, &v.SubmittedAt, &v.UpdatedAt, &v.ItemId, &v.Weight, &v.Flags)
//		if err != nil {
//			return nil, err
//		}
//		for key, vv := range a.votes {
//			if vv.id == v.Id {
//				a.votes[key] = Vote{
//					SubmittedBy: a.Handle,
//					SubmittedAt: v.SubmittedAt,
//					UpdatedAt:   v.UpdatedAt,
//					ItemHash:    hashes[key], // FIXME(marius): v.Id
//					Weight:      v.Weight,
//					id:          v.Id,
//					flags:       v.Flags,
//					itemId:      v.ItemId,
//				}
//				continue RowLoop
//			}
//		}
//		a.votes = append(a.votes, Vote{
//			SubmittedBy: a.Handle,
//			SubmittedAt: v.SubmittedAt,
//			UpdatedAt:   v.UpdatedAt,
//			ItemHash:    hashes[0], // FIXME(marius): v.Id
//			Weight:      v.Weight,
//			id:          v.Id,
//			flags:       v.Flags,
//			itemId:      v.ItemId,
//		})
//	}
//	return a.votes, nil
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
	loader, ok := val.(models.CanLoad)
	if ok {
		log.Infof("loaded LoaderService of type %T", loader)
	} else {
		log.Errorf("could not load loader service from Context")
		return
	}

	var err error
	items, err := loader.LoadItems(models.LoadItemsFilter{MaxItems:MaxContentItems})
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}

	ShowItemData = false
	m.Items = loadComments(items)

	_, err = LoadVotes(CurrentAccount, m.Items.getItems())
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}

	RenderTemplate(r, w, "listing", m)
}
