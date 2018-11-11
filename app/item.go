package app

import (
	"strings"
	"time"

	"github.com/juju/errors"
)

type ItemMetadata []byte

type Identifiable interface {
	Id() int64
}

type Item struct {
	Hash        Hash         `json:"key"`
	Title       string       `json:"title"`
	MimeType    string       `json:"mimeType"`
	Data        string       `json:"data"`
	Score       int64        `json:"score"`
	SubmittedAt time.Time    `json:"createdAt"`
	SubmittedBy *Account     `json:"submittedBy"`
	UpdatedAt   time.Time    `json:"-"`
	Flags       FlagBits     `json:"-"`
	Path        []byte       `json:"-"`
	FullPath    []byte       `json:"-"`
	Metadata    ItemMetadata `json:"-"`
	IsTop       bool         `json:"isTop"`
	Parent      *Item        `json:"-"`
	OP          *Item        `json:"-"`
}

func (i Item) Deleted() bool {
	return (i.Flags & FlagsDeleted) == FlagsDeleted
}

func (i Item) UnDelete() {
	i.Flags ^= FlagsDeleted
}
func (i *Item) Delete() {
	i.Flags &= FlagsDeleted
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
	mimeComponents := strings.Split(i.MimeType, "/")
	return mimeComponents[0] == "text"
}

func (i ItemCollection) First() (*Item, error) {
	for _, it := range i {
		return &it, nil
	}
	return nil, errors.Errorf("empty %T", i)
}
