package app

import (
	"bytes"
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

var ValidContentTypes = pub.ActivityVocabularyTypes{
	pub.ArticleType,
	pub.NoteType,
	pub.LinkType,
	pub.PageType,
	pub.DocumentType,
	pub.VideoType,
	pub.AudioType,
}

var ValidContentManagementTypes = pub.ActivityVocabularyTypes{
	pub.UpdateType,
	pub.CreateType,
	pub.DeleteType,
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

func updateItemFromRequest(r *http.Request, author Account, i *Item) error {
	if r.Method != http.MethodPost {
		return errors.Errorf("invalid http method type")
	}

	var receivers []Account
	var err error

	if i.Metadata == nil {
		i.Metadata = new(ItemMetadata)
	}
	if hash := HashFromString(r.PostFormValue("hash")); hash.IsValid() {
		i.Hash = hash
	}
	if receivers, err = accountsFromRequestHandle(r); err == nil && chi.URLParam(r, "hash") == "" {
		i.MakePrivate()
		for _, rec := range receivers {
			if !rec.IsValid() {
				continue
			}
			i.Metadata.To = append(i.Metadata.To, rec)
		}
	}
	if tit := r.PostFormValue("title"); len(tit) > 0 {
		i.Title = tit
	}
	if dat := r.PostFormValue("data"); len(dat) > 0 {
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
	if parent := HashFromString(r.PostFormValue("parent")); parent.IsValid() {
		if i.Parent == nil || i.Parent.Hash != parent {
			i.Parent = &Item{Hash: parent}
		}
	}
	if op := HashFromString(r.PostFormValue("op")); op.IsValid() {
		if i.OP != nil || i.OP.Hash != op {
			i.OP = &Item{Hash: op}
		}
	}
	return nil
}

func ContentFromRequest(r *http.Request, author Account) (Item, error) {
	i := Item{}
	return i, updateItemFromRequest(r, author, &i)
}
