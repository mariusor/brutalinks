package app

import (
	"fmt"
	"time"

	"github.com/juju/errors"
)

type SSHKey struct {
	ID      string `json:"id"`
	Private []byte `json:"prv,omitempty"`
	Public  []byte `json:"pub,omitempty"`
}

type ImageMetadata struct {
	Path     []byte `json:"path,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

type AccountMetadata struct {
	Password  []byte        `json:"pw,omitempty"`
	Provider  string        `json:"provider,omitempty"`
	Salt      []byte        `json:"salt,omitempty"`
	Key       *SSHKey       `json:"key,omitempty"`
	Blurb     []byte        `json:"blurb,omitempty"`
	Avatar    ImageMetadata `json:"avatar,omitempty"`
	ID        string        `json:"id,omitempty"`
	URL       string        `json:"url,omitempty"`
	InboxIRI  string        `json:"inbox,omitempty"`
	OutboxIRI string        `json:"outbox,omitempty"`
}

type AccountCollection []Account

type Account struct {
	Email     string           `json:"email,omitempty"`
	Hash      Hash             `json:"hash,omitempty"`
	Score     int64            `json:"score,omitempty"`
	Handle    string           `json:"handle,omitempty"`
	CreatedAt time.Time        `json:"-"`
	UpdatedAt time.Time        `json:"-"`
	Flags     FlagBits         `json:"flags,omitempty"`
	Metadata  *AccountMetadata `json:"-"`
	Votes     VoteCollection   `json:"votes,omitempty"`
}

// Hash
type Hash string

// String
func (h Hash) String() string {
	if len(h) > 8 {
		return string(h[0:8])
	} else {
		return string(h)
	}
}

// MarshalText
func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h[0:8]), nil
}

// HasMetadata
func (a Account) HasMetadata() bool {
	return a.Metadata != nil
}

// HasPublicKey
func (a Account) HasPublicKey() bool {
	return a.HasMetadata() && a.Metadata.Key != nil && a.Metadata.Key.Public != nil
}

func (a Account) IsValid() bool {
	return len(a.Handle) > 0 || len(a.Hash) > 0
}

type Deletable interface {
	Deleted() bool
	Delete()
	UnDelete()
}

func (a Account) VotedOn(i Item) *Vote {
	for key, v := range a.Votes {
		if key == i.Hash {
			return &v
		}
	}
	return nil
}

func (a Account) GetLink() string {
	return fmt.Sprintf("/~%s", a.Handle)
}

func (a Account) IsLogged() bool {
	return !a.CreatedAt.IsZero()
}

func (a AccountCollection) First() (*Account, error) {
	for _, act := range a {
		return &act, nil
	}
	return nil, errors.Errorf("empty %T", a)
}
