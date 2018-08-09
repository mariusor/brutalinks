package app

import (
	"bytes"
	"fmt"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"github.com/juju/errors"
	"strings"
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

func LoadVotes(a *models.Account, it models.ItemCollection) (map[string]models.Vote, error) {
	var ids = make([]int64, len(it))
	var hashes = make(map[string]string, 0)
	for i, k := range it {
		ids[i] = int64(i)
		hashes[k.Hash] = k.Hash
	}
	if a == nil {
		return nil, errors.Errorf("no account to load for")
	}
	if len(ids) == 0 {
		log.Error(errors.Errorf("no ids to load"))
	}
	// this here code following is the ugliest I wrote in quite a long time
	// so ugly it warrants its own fucking shame corner
	sids := make([]string, 0)
	for i := 0; i < len(ids); i++ {
		sids = append(sids, fmt.Sprintf("$%d", i+2))
	}
	iitems := make([]interface{}, len(ids)+1)
	iitems[0] = a.Hash
	for i, v := range ids {
		iitems[i+1] = v
	}
	sel := fmt.Sprintf(`select "votes"."id", "content_items"."key", "votes"."submitted_by", 
		"votes"."submitted_at", "votes"."updated_at", "item_id", "weight", "votes"."flags"
	from "votes" 
	inner join "content_items" on "content_items"."id" = "votes"."item_id"
	inner join "accounts" on "accounts"."id" = "votes"."submitted_by"
	where "content_items"."key" = $1 and "votes"."item_id" in (%s)`, strings.Join(sids, ", "))
	rows, err := Db.Query(sel, iitems...)

	//log.Debugf("q: %s", sel)
	//log.Debugf("q: %#v", ids)
	if err != nil {
		return nil, err
	}
	if a.Votes == nil {
		a.Votes = make(map[string]models.Vote, 0)
	}
RowLoop:
	for rows.Next() {
		v := models.Vote{}
		var vId int64
		var vHash string
		var vItemId int64
		err = rows.Scan(&vId, &vHash, &v.SubmittedBy, &v.SubmittedAt, &v.UpdatedAt, &vItemId, &v.Weight, &v.Flags)
		if err != nil {
			return nil, err
		}
		for key, vv := range a.Votes {
			if vv.Item.Hash == vHash {
				log.Debugf("checking %s vs %s", vv.Item.Hash, vHash)
				a.Votes[key] = models.Vote{
					SubmittedBy: a,
					SubmittedAt: v.SubmittedAt,
					UpdatedAt:   v.UpdatedAt,
					Item:        vv.Item, // FIXME(marius): v.Id
					Weight:      v.Weight,
					Flags:       v.Flags,
				}
				continue RowLoop
			}
		}
		a.Votes[vHash] = models.Vote{
			SubmittedBy: a,
			SubmittedAt: v.SubmittedAt,
			UpdatedAt:   v.UpdatedAt,
			Weight:      v.Weight,
			Flags:       v.Flags,
		}
	}
	return a.Votes, nil
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
	items, err := itemLoader.LoadItems(models.LoadItemsFilter{MaxItems: MaxContentItems, })
	if err != nil {
		log.Error(err)
		HandleError(w, r, http.StatusNotFound, err)
		return
	}

	ShowItemData = false
	m.Items = loadComments(items)

	votesLoader, ok := val.(models.CanLoadVotes)
	votes, err := votesLoader.LoadVotes(models.LoadVotesFilter{
		SubmittedBy: []string{CurrentAccount.Hash,},
		ItemKey: m.Items.getItemsHashes(),
	})
	for _, v := range votes {
		CurrentAccount.Votes[v.Item.Hash] = v
	}
	_, err = models.LoadAccountVotes(CurrentAccount, m.Items.getItems())
	//_, err = LoadVotes(CurrentAccount, m.Items.getItems())
	if err != nil {
		log.Error(err)
		//HandleError(w, r, http.StatusNotFound, err)
		return
	}

	RenderTemplate(r, w, "listing", m)
}
