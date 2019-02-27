package app

import (
	"strings"
	"time"

	"github.com/mariusor/littr.go/internal/errors"
)

type Tag struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"id,omitempty"`
}

type TagCollection []Tag

type ItemMetadata struct {
	Tags       TagCollection `json:"tags,omitempty"`
	Mentions   TagCollection `json:"mentions,omitempty"`
	ID         string        `json:"id,omitempty"`
	URL        string        `json:"url,omitempty"`
	RepliesURI string        `json:"replies,omitempty"`
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

type Item struct {
	Hash        Hash          `json:"hash"`
	Title       string        `json:"-"`
	MimeType    MimeType      `json:"-"`
	Data        string        `json:"-"`
	Score       int64         `json:"-"`
	SubmittedAt time.Time     `json:"-"`
	SubmittedBy *Account      `json:"-"`
	UpdatedAt   time.Time     `json:"-"`
	Flags       FlagBits      `json:"-"`
	Path        []byte        `json:"-"`
	FullPath    []byte        `json:"-"`
	Metadata    *ItemMetadata `json:"-"`
	IsTop       bool          `json:"-"`
	Parent      *Item         `json:"-"`
	OP          *Item         `json:"-"`
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

func (i Item) IsLink() bool {
	return i.MimeType == MimeTypeURL
}

func (i Item) GetDomain() string {
	if !i.IsLink() {
		return ""
	}
	return strings.Split(i.Data, "/")[2]
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
