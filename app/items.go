package app

import (
	"bytes"
	"context"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-chi/chi"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-ap/errors"
	"github.com/mariusor/littr.go/internal/log"
)

const TagMention = "mention"
const TagTag = "tag"

type Tag struct {
	Type string `json:"-"`
	Name string `json:"name,omitempty"`
	URL  string `json:"id,omitempty"`
}

type TagCollection []Tag

type ItemMetadata struct {
	To         []*Account    `json:"to,omitempty"`
	CC         []*Account    `json:"to,omitempty"`
	Tags       TagCollection `json:"tags,omitempty"`
	Mentions   TagCollection `json:"mentions,omitempty"`
	ID         string        `json:"id,omitempty"`
	URL        string        `json:"url,omitempty"`
	RepliesURI string        `json:"replies,omitempty"`
	LikesURI   string        `json:"likes,omitempty"`
	SharesURI  string        `json:"shares,omitempty"`
	AuthorURI  string        `json:"author,omitempty"`
	Icon       ImageMetadata `json:"icon,omitempty"`
}

type Identifiable interface {
	Id() int64
}

type data struct {
	Source    string
	Processed string
}

func (d data) String() string {
	return d.Processed
}

func (i *Item) IsValid() bool {
	return i != nil && len(i.Hash) > 0
}

func (i Item) Deleted() bool {
	return (i.Flags & FlagsDeleted) == FlagsDeleted
}

// UnDelete remove the deleted flag from an item
func (i Item) UnDelete() {
	i.Flags ^= FlagsDeleted
}

// Delete add the deleted flag on an item
func (i *Item) Delete() {
	i.Flags |= FlagsDeleted
}

func (i Item) Private() bool {
	return (i.Flags & FlagsPrivate) == FlagsPrivate
}

func (i Item) Public() bool {
	return (i.Flags & FlagsPrivate) != FlagsPrivate
}

func (i *Item) MakePrivate() {
	i.Flags |= FlagsPrivate
}

func (i *Item) MakePublic() {
	i.Flags ^= FlagsPrivate
}

func (i Item) IsLink() bool {
	return i.MimeType == MimeTypeURL
}

func (i Item) GetDomain() string {
	if !i.IsLink() {
		return ""
	}
	u, err := url.Parse(i.Data)
	if err == nil && len(u.Host) > 0 {
		return u.Host
	}
	return "unknown"
}

func (i Item) IsSelf() bool {
	mimeComponents := strings.Split(string(i.MimeType), "/")
	return mimeComponents[0] == "text"
}

func (i ItemCollection) First() (*Item, error) {
	for _, it := range i {
		return &it, nil
	}
	return nil, errors.Errorf("empty %T", i)
}

const (
	MaxContentItems = 50
)

func loadItems(c context.Context, filter Filters, acc *Account, l log.Logger) (ItemCollection, error) {
	repo := ContextRepository(c)
	if repo == nil {
		err := errors.Errorf("could not load item repository from Context")
		return nil, err
	}
	contentItems, _, err := repo.LoadItems(filter)

	if err != nil {
		return nil, err
	}
	if acc.IsLogged() {
		acc.Votes, _, err = repo.LoadVotes(Filters{
			LoadVotesFilter: LoadVotesFilter{
				AttributedTo: []Hash{acc.Hash},
				ItemKey:      contentItems.getItemsHashes(),
			},
			MaxItems: MaxContentItems,
		})
		if err != nil {
			l.Error(err.Error())
		}
	}
	return contentItems, nil
}

func detectMimeType(data string) MimeType {
	u, err := url.ParseRequestURI(data)
	if err == nil && u != nil && !bytes.ContainsRune([]byte(data), '\n') {
		return MimeTypeURL
	}
	return "text/plain"
}

func getTagFromBytes(d []byte) Tag {
	var name, host []byte
	t := Tag{}

	if ind := bytes.LastIndex(d, []byte{'@'}); ind > 1 {
		name = d[1:ind]
		host = []byte(fmt.Sprintf("https://%s", d[ind+1:]))
	} else {
		name = d[1:]
		host = []byte(Instance.BaseURL)
	}
	if d[0] == '@' || d[0] == '~' {
		// mention
		t.Type = TagMention
		t.Name = string(name)
		t.URL = fmt.Sprintf("%s/~%s", host, name)
	}
	if d[0] == '#' {
		// @todo(marius) :link_generation: make the tag links be generated from the corresponding route
		t.Type = TagTag
		t.Name = string(name)
		t.URL = fmt.Sprintf("%s/t/%s", host, name)
	}
	return t
}

func loadTags(data string) (TagCollection, TagCollection) {
	if !strings.ContainsAny(data, "#@~") {
		return nil, nil
	}
	tags := make(TagCollection, 0)
	mentions := make(TagCollection, 0)

	r := regexp.MustCompile(`(?:\A|\s)((?:[~@]\w+)(?:@\w+.\w+)?|(?:#\w{4,}))`)
	matches := r.FindAllSubmatch([]byte(data), -1)

	for _, sub := range matches {
		t := getTagFromBytes(sub[1])
		if t.Type == TagMention {
			mentions = append(mentions, t)
		}
		if t.Type == TagTag {
			tags = append(tags, t)
		}
	}
	return tags, mentions
}

func ContentFromRequest(r *http.Request, acc Account) (Item, error) {
	if r.Method != http.MethodPost {
		return Item{}, errors.Errorf("invalid http method type")
	}

	var receiver *Account
	var err error
	i := Item{}
	i.Metadata = &ItemMetadata{}
	if receiver, err = accountFromRequestHandle(r); err == nil && chi.URLParam(r, "hash") == "" {
		i.MakePrivate()
		to := Account{}
		to.FromActivityPub(pub.IRI(receiver.Metadata.ID))
		i.Metadata.To = []*Account{&to}
	}

	tit := r.PostFormValue("title")
	if len(tit) > 0 {
		i.Title = tit
	}
	dat := r.PostFormValue("data")
	if len(dat) > 0 {
		i.Data = dat
	}

	i.SubmittedBy = &acc
	i.MimeType = detectMimeType(i.Data)

	i.Metadata.Tags, i.Metadata.Mentions = loadTags(i.Data)
	if !i.IsLink() {
		i.MimeType = MimeType(r.PostFormValue("mime-type"))
	}
	if len(i.Data) > 0 {
		now := time.Now().UTC()
		i.SubmittedAt = now
		i.UpdatedAt = now
	}
	parent := r.PostFormValue("parent")
	if len(parent) > 0 {
		i.Parent = &Item{Hash: Hash(parent)}
	}
	op := r.PostFormValue("op")
	if len(op) > 0 {
		i.OP = &Item{Hash: Hash(op)}
	}
	hash := r.PostFormValue("hash")
	if len(hash) > 0 {
		i.Hash = Hash(hash)
	}
	return i, nil
}
