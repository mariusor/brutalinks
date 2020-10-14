package app

import (
	"bytes"
	"context"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-chi/chi"
	"github.com/pborman/uuid"
	"golang.org/x/oauth2"
	"net/http"
	"time"

	"github.com/go-ap/errors"
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

type OAuth struct {
	Provider string
	Code     string
	Token    *oauth2.Token
	State    string
}

type AccountMetadata struct {
	Password      []byte        `json:"pw,omitempty"`
	Provider      string        `json:"provider,omitempty"`
	Salt          []byte        `json:"salt,omitempty"`
	Key           *SSHKey       `json:"key,omitempty"`
	Blurb         []byte        `json:"blurb,omitempty"`
	Icon          ImageMetadata `json:"icon,omitempty"`
	Name          string        `json:"name,omitempty"`
	ID            string        `json:"id,omitempty"`
	URL           string        `json:"url,omitempty"`
	InboxIRI      string        `json:"inbox,omitempty"`
	OutboxIRI     string        `json:"outbox,omitempty"`
	LikedIRI      string        `json:"liked,omitempty"`
	FollowersIRI  string        `json:"followers,omitempty"`
	FollowingIRI  string        `json:"following,omitempty"`
	OAuth         OAuth         `json:-`
	outboxUpdated time.Time
	outbox        pub.ItemCollection
}

type AccountCollection []Account

type Account struct {
	Email     string               `json:"-"`
	Hash      Hash                 `json:"hash,omitempty"`
	Handle    string               `json:"handle,omitempty"`
	CreatedAt time.Time            `json:"-"`
	CreatedBy *Account             `json:"-"`
	UpdatedAt time.Time            `json:"-"`
	Flags     FlagBits             `json:"flags,omitempty"`
	Metadata  *AccountMetadata     `json:"-"`
	pub       pub.Item             `json:"-"`
	Votes     VoteCollection       `json:"-"`
	Followers AccountCollection    `json:"followers,omitempty"`
	Following AccountCollection    `json:"following,omitempty"`
	Blocked   AccountCollection    `json:"-"`
	Ignored   AccountCollection    `json:"-"`
	Level     uint8                `json:"-"`
	Parent    *Account             `json:"-"`
	Children  AccountPtrCollection `json:"-"`
}

// Hash is a local type for string, it should hold a [32]byte array actually
type Hash uuid.UUID

func HashesEqual(h1, h2 Hash) bool {
	return uuid.Equal(uuid.UUID(h1), uuid.UUID(h2))
}

// String returns the hash as a string
func (h Hash) String() string {
	return string(h)
}

// Short returns a minimal valuable string value
func (h Hash) Short() string {
	//if len(h) > 10 {
	//	return string(h[0:10])
	//}
	return string(h)
}

// MarshalText
func (h Hash) MarshalText() ([]byte, error) {
	return h, nil
}

// HasMetadata
func (a *Account) HasMetadata() bool {
	return a != nil && a.Metadata != nil
}

// IsFederated
func (a *Account) IsFederated() bool {
	return !a.IsLocal()
}

// IsLocal
func (a *Account) IsLocal() bool {
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
func (a *Account) HasPublicKey() bool {
	return a.HasMetadata() && a.Metadata.Key != nil && len(a.Metadata.Key.Public) > 0
}

// IsValid returns if the current account has a handle or a hash with length greater than 0
func (a *Account) IsValid() bool {
	return a != nil && len(a.Hash) > 0
}

// AP returns the underlying actvitypub item
func (a *Account) AP() pub.Item {
	return a.pub
}

// Private
func (a *Account) Private() bool {
	return a != nil && a.Flags&FlagsPrivate == FlagsPrivate
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
		if bytes.Equal(v.Item.Hash, i.Hash) {
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
func (a *Account) IsLogged() bool {
	return a != nil && (!a.CreatedAt.IsZero() || (!bytes.Equal(a.Hash, AnonymousHash) && a.Handle != Anonymous))
}

// HasIcon
func (a *Account) HasIcon() bool {
	return a.HasMetadata() && len(a.Metadata.Icon.URI) > 0
}

// Deleted
func (a *Account) Deleted() bool {
	return a != nil && (a.Flags&FlagsDeleted) == FlagsDeleted
}

func (a Account) Type() RenderType {
	return ActorType
}

func (a Account) Date() time.Time {
	return a.CreatedAt
}

// First
func (a AccountCollection) First() (*Account, error) {
	for _, act := range a {
		return &act, nil
	}
	return nil, errors.Errorf("empty %T", a)
}

func accountFromPost(r *http.Request) (Account, error) {
	if r.Method != http.MethodPost {
		return AnonymousAccount, errors.Errorf("invalid http method type")
	}

	var a *Account
	var err error
	hash := r.PostFormValue("hash")
	if len(hash) > 0 {
		// NOTE(marius): coming from an invite
		s := ContextRepository(r.Context())
		a, err = s.LoadAccount(context.Background(), ActorsURL.AddPath(hash))
	}
	if accountsEqual(*a, AnonymousAccount) || err != nil {
		*a = Account{Metadata: &AccountMetadata{}}
	}
	pw := r.PostFormValue("pw")
	pwConfirm := r.PostFormValue("pw-confirm")
	if pw != pwConfirm {
		return AnonymousAccount, errors.Errorf("the passwords don't match")
	}

	/*
		agree := r.PostFormValue("agree")
		if agree != "y" {
			errs = append(errs, errors.Errorf("you must agree not to be a dick to other people"))
		}
	*/
	handle := r.PostFormValue("handle")
	if len(handle) > 0 {
		a.Handle = handle
	}
	a.Metadata = &AccountMetadata{
		Password: []byte(pw),
	}
	return *a, nil
}

func accountsFromRequestHandle(r *http.Request) ([]*Account, error) {
	handle := chi.URLParam(r, "handle")
	if handle == "" {
		return nil, errors.NotFoundf("missing account handle %s", handle)
	}
	fa := &Filters{
		Name: CompStrs{EqualsString(handle)},
	}
	repo := ContextRepository(r.Context())
	return repo.accounts(context.Background(), fa)
}

type AccountPtrCollection []*Account

func (h AccountPtrCollection) Contains(s Hash) bool {
	for _, hh := range h {
		if HashesEqual(hh.Hash, s) {
			return true
		}
	}
	return false
}

func addLevelAccounts(allAccounts AccountPtrCollection) {
	if len(allAccounts) == 0 {
		return
	}
	leveled := make(Hashes, 0)
	var setLevel func(AccountPtrCollection)

	setLevel = func(com AccountPtrCollection) {
		for _, cur := range com {
			if cur == nil || leveled.Contains(cur.Hash) {
				break
			}
			leveled = append(leveled, cur.Hash)
			if len(cur.Children) > 0 {
				for _, child := range cur.Children {
					child.Level = cur.Level + 1
					setLevel(cur.Children)
				}
			}
		}
	}
	setLevel(allAccounts)
}

func reparentAccounts(allAccounts *AccountPtrCollection) {
	if len(*allAccounts) == 0 {
		return
	}
	parFn := func(t AccountPtrCollection, cur *Account) *Account {
		for _, n := range t {
			if cur.CreatedBy.IsValid() {
				if HashesEqual(cur.CreatedBy.Hash, n.Hash) {
					return n
				}
			}
		}
		return nil
	}

	retAccounts := make(AccountPtrCollection, 0)
	for _, cur := range *allAccounts {
		if par := parFn(*allAccounts, cur); par != nil {
			par.Children = append(par.Children, cur)
			cur.Parent = par
		} else {
			retAccounts = append(retAccounts, cur)
		}
	}
	*allAccounts = retAccounts
}
