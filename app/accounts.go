package app

import (
	"bytes"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-chi/chi"
	"github.com/pborman/uuid"
	"net/http"
	"time"

	"github.com/go-ap/errors"
	"github.com/mariusor/littr.go/internal/log"
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
	Provider     string
	Code         string
	Token        string
	RefreshToken string
	TokenType    string
	Expiry       time.Time
	State        string
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
	OAuth        OAuth         `json:-`
}

type AccountCollection []Account

type Account struct {
	Email     string            `json:"email,omitempty"`
	Hash      Hash              `json:"hash,omitempty"`
	Score     int               `json:"score,omitempty"`
	Handle    string            `json:"handle,omitempty"`
	CreatedAt time.Time         `json:"-"`
	CreatedBy *Account          `json:"-"`
	UpdatedAt time.Time         `json:"-"`
	Flags     FlagBits          `json:"flags,omitempty"`
	Metadata  *AccountMetadata  `json:"-"`
	pub       pub.Item          `json:"-"`
	Votes     VoteCollection    `json:"votes,omitempty"`
	Followers AccountCollection `json:"followers,omitempty"`
	Following AccountCollection `json:"following,omitempty"`
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
func (a *Account) IsValid() bool {
	return a != nil && len(a.Hash) > 0
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
	return a != nil && !a.CreatedAt.IsZero() || (!bytes.Equal(a.Hash, AnonymousHash) && a.Handle != Anonymous)
}

// HasIcon
func (a Account) HasIcon() bool {
	return a.HasMetadata() && len(a.Metadata.Icon.URI) > 0
}

// Deleted
func (a Account) Deleted() bool {
	return (a.Flags & FlagsDeleted) == FlagsDeleted
}

// First
func (a AccountCollection) First() (*Account, error) {
	for _, act := range a {
		return &act, nil
	}
	return nil, errors.Errorf("empty %T", a)
}

func accountFromPost(r *http.Request, l log.Logger) (*Account, error) {
	if r.Method != http.MethodPost {
		return nil, errors.Errorf("invalid http method type")
	}

	a := Account{Metadata: &AccountMetadata{}}
	pw := r.PostFormValue("pw")
	pwConfirm := r.PostFormValue("pw-confirm")
	if pw != pwConfirm {
		return nil, errors.Errorf("the passwords don't match")
	}

	/*
		agree := r.PostFormValue("agree")
		if agree != "y" {
			errs = append(errs, errors.Errorf("you must agree not to be a dick to other people"))
		}
	*/
	handle := r.PostFormValue("handle")
	if handle != "" {
		a.Handle = handle
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now

	a.Metadata = &AccountMetadata{
		Password: []byte(pw),
	}

	return &a, nil
}

func checkUserCreatingEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Instance.Config.UserCreatingEnabled {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
		next.ServeHTTP(w, r)
	})
}

func accountFromRequestHandle(r *http.Request) (*Account, error) {
	handle := chi.URLParam(r, "handle")
	if handle == "" {
		return nil, errors.NotFoundf("missing account handle %s", handle)
	}
	fa := &Filters{
		Name: CompStrs{EqualsString(handle)},
	}
	repo := ContextRepository(r.Context())
	var account *Account
	if accounts, err := repo.accounts(fa); err == nil {
		if len(accounts) == 0 {
			return nil, errors.NotFoundf("account %q not found", handle)
		}
		if len(accounts) > 1 {
			return nil, errors.NotFoundf("too many %q accounts found", handle)
		}
		account = &accounts[0]
	}
	return account, nil
}
