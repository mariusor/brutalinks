package app

import (
	"fmt"
	"time"

	"github.com/mariusor/littr.go/internal/errors"
)

type SSHKey struct {
	ID      string `json:"id"`
	Private []byte `json:"prv,omitempty"`
	Public  []byte `json:"pub,omitempty"`
}

type ImageMetadata struct {
	URI      string `json:"uri,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

type AccountMetadata struct {
	Password     []byte        `json:"pw,omitempty"`
	Provider     string        `json:"provider,omitempty"`
	Salt         []byte        `json:"salt,omitempty"`
	Key          *SSHKey       `json:"key,omitempty"`
	Blurb        []byte        `json:"blurb,omitempty"`
	Icon         ImageMetadata `json:"icon,omitempty"`
	Name         string        `json:"name,omitempty"`
	ID           string        `json:"id,omitempty"`
	URL          string        `json:"url,omitempty"`
	InboxIRI     string        `json:"inbox,omitempty"`
	OutboxIRI    string        `json:"outbox,omitempty"`
	LikedIRI     string        `json:"liked,omitempty"`
	FollowersIRI string        `json:"followers,omitempty"`
	FollowingIRI string        `json:"following,omitempty"`
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
	if len(h) > 8 {
		return []byte(h[0:8]), nil
	}
	return []byte(h), nil
}

// HasMetadata
func (a Account) HasMetadata() bool {
	return a.Metadata != nil
}

// IsFederated
func (a Account) IsFederated() bool {
	return !a.IsLocal()
}

// IsLocal
func (a Account) IsLocal() bool {
	if !a.HasMetadata() {
		return true
	}
	if len(a.Metadata.ID) > 0 {
		return HostIsLocal(a.Metadata.ID)
	}
	if len(a.Metadata.URL) > 0 {
		return HostIsLocal(a.Metadata.URL)
	}
	return true
}

// HasPublicKey returns if current account had a public ssh key generated
func (a Account) HasPublicKey() bool {
	return a.HasMetadata() && a.Metadata.Key != nil && len(a.Metadata.Key.Public) > 0
}

// IsValid returns if the current account has a handle or a hash with length greater than 0
func (a Account) IsValid() bool {
	return len(a.Handle) > 0 || len(a.Hash) > 0
}

// Deletable
type Deletable interface {
	Deleted() bool
	Delete()
	UnDelete()
}

func (a Account) VotedOn(i Item) *Vote {
	for _, v := range a.Votes {
		if v.Item == nil {
			continue
		}
		if v.Item.Hash == i.Hash {
			return &v
		}
	}
	return nil
}

func (a Account) GetLink() string {
	if a.IsLocal() {
		return fmt.Sprintf("/~%s", a.Handle)
	}
	return a.Metadata.URL
}

// IsLogged should show if current user was loaded from a session
func (a Account) IsLogged() bool {
	return !a.CreatedAt.IsZero()
}

// HasIcon
func (a Account) HasIcon() bool {
	return a.HasMetadata() && len(a.Metadata.Icon.URI) > 0
}

// First
func (a AccountCollection) First() (*Account, error) {
	for _, act := range a {
		return &act, nil
	}
	return nil, errors.Errorf("empty %T", a)
}
