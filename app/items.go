package app

import (
	"bytes"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-chi/chi"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-ap/errors"
)

type ItemMetadata struct {
	To         AccountCollection `json:"to,omitempty"`
	CC         AccountCollection `json:"to,omitempty"`
	Tags       TagCollection     `json:"tags,omitempty"`
	Mentions   TagCollection     `json:"mentions,omitempty"`
	ID         string            `json:"id,omitempty"`
	URL        string            `json:"url,omitempty"`
	RepliesURI string            `json:"replies,omitempty"`
	LikesURI   string            `json:"likes,omitempty"`
	SharesURI  string            `json:"shares,omitempty"`
	AuthorURI  string            `json:"author,omitempty"`
	Icon       ImageMetadata     `json:"icon,omitempty"`
}

type Identifiable interface {
	Id() int64
}

func (i *Item) IsValid() bool {
	return i != nil && i.Hash.IsValid()
}

// AP returns the underlying actvitypub item
func (i *Item) AP() pub.Item {
	return i.pub
}

// Content returns the content of the Item
func (i Item) Content() map[string][]byte {
	return map[string][]byte{i.MimeType: []byte(i.Data)}
}

// Tags returns the tags associated with the current Item
func (i Item) Tags() TagCollection {
	return i.Metadata.Tags
}

// Mentions returns the mentions associated with the current Item
func (i Item) Mentions() TagCollection {
	return i.Metadata.Mentions
}

func (i *Item) Deleted() bool {
	return i != nil && (i.Flags&FlagsDeleted) == FlagsDeleted
}

// UnDelete remove the deleted flag from an item
func (i *Item) UnDelete() {
	i.Flags ^= FlagsDeleted
}

// Delete add the deleted flag on an item
func (i *Item) Delete() {
	i.Flags |= FlagsDeleted
}

func (i *Item) Private() bool {
	return i != nil && (i.Flags&FlagsPrivate) == FlagsPrivate
}

func (i *Item) Public() bool {
	return i != nil && (i.Flags&FlagsPrivate) != FlagsPrivate
}

func (i *Item) MakePrivate() {
	i.Flags |= FlagsPrivate
}

func (i *Item) MakePublic() {
	i.Flags ^= FlagsPrivate
}

func (i *Item) IsLink() bool {
	return i != nil && i.MimeType == MimeTypeURL
}

const unknownDomain = "unknown"
const githubDomain = "github.com"

func getDomain(u *url.URL) string {
	if u == nil || len(u.Host) == 0 {
		return unknownDomain
	}
	switch u.Host {
	case githubDomain:
		pathEl := strings.Split(strings.TrimLeft(u.Path, "/"), "/")
		if len(pathEl) > 0 {
			return fmt.Sprintf("%s/%s", u.Host, pathEl[0])
		}
	}
	return u.Host
}

func (i Item) GetDomain() string {
	if !i.IsLink() {
		return ""
	}
	u, err := url.Parse(i.Data)
	if err != nil {
		return unknownDomain
	}
	return getDomain(u)
}

func (i Item) IsSelf() bool {
	mimeComponents := strings.Split(i.MimeType, "/")
	return mimeComponents[0] == "text"
}

func (i ItemCollection) First() (*Item, error) {
	for _, it := range i {
		return &it, nil
	}
	return nil, errors.Errorf("empty %T", i)
}

const (
	MaxContentItems = 35
)

func detectMimeType(data string) string {
	u, err := url.ParseRequestURI(data)
	if err == nil && u != nil && !bytes.ContainsRune([]byte(data), '\n') {
		return MimeTypeURL
	}
	return "text/plain"
}

func ContentFromRequest(r *http.Request, author Account) (Item, error) {
	if r.Method != http.MethodPost {
		return Item{}, errors.Errorf("invalid http method type")
	}

	var receivers []Account
	var err error
	i := Item{}
	i.Metadata = &ItemMetadata{}
	if receivers, err = accountsFromRequestHandle(r); err == nil && chi.URLParam(r, "hash") == "" {
		i.MakePrivate()
		for _, rec := range receivers {
			if !rec.IsValid() {
				continue
			}
			i.Metadata.To = append(i.Metadata.To, rec)
		}
	}

	tit := r.PostFormValue("title")
	if len(tit) > 0 {
		i.Title = tit
	}
	dat := r.PostFormValue("data")
	if len(dat) > 0 {
		i.Data = dat
	}

	i.SubmittedBy = &author
	i.MimeType = detectMimeType(i.Data)

	i.Metadata.Tags, i.Metadata.Mentions = loadTags(i.Data)
	if !i.IsLink() {
		i.MimeType = r.PostFormValue("mime-type")
	}
	if len(i.Data) > 0 {
		now := time.Now().UTC()
		i.SubmittedAt = now
		i.UpdatedAt = now
	}
	parent := r.PostFormValue("parent")
	if len(parent) > 0 {
		i.Parent = &Item{Hash: HashFromString(parent)}
	}
	op := r.PostFormValue("op")
	if len(op) > 0 {
		i.OP = &Item{Hash: HashFromString(op)}
	}
	hash := r.PostFormValue("hash")
	if len(hash) > 0 {
		i.Hash = HashFromString(hash)
	}
	return i, nil
}
