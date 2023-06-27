package brutalinks

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi/v5"
	"golang.org/x/oauth2"
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
	Password              []byte               `json:"pw,omitempty"`
	Key                   *SSHKey              `json:"key,omitempty"`
	Blurb                 string               `json:"blurb,omitempty"`
	Icon                  ImageMetadata        `json:"icon,omitempty"`
	Name                  string               `json:"name,omitempty"`
	ID                    string               `json:"id,omitempty"`
	URL                   string               `json:"url,omitempty"`
	Tags                  TagCollection        `json:"tags,omitempty"`
	InboxIRI              string               `json:"inbox,omitempty"`
	OutboxIRI             string               `json:"outbox,omitempty"`
	LikedIRI              string               `json:"liked,omitempty"`
	FollowersIRI          string               `json:"followers,omitempty"`
	FollowingIRI          string               `json:"following,omitempty"`
	OAuth                 OAuth                `json:"-"`
	AuthorizationEndPoint string               `json:"authorizationEndPoint,omitempty"`
	TokenEndPoint         string               `json:"tokenEndPoint,omitempty"`
	OutboxUpdated         time.Time            `json:"outboxUpdated,omitempty"`
	Outbox                vocab.ItemCollection `json:"-"`
}

func (m *AccountMetadata) InvalidateOutbox() {
	m.OutboxUpdated = time.Time{}
	m.Outbox = m.Outbox[:0]
}

type AccountCollection []Account

type Account struct {
	Hash      Hash              `json:"hash,omitempty"`
	Handle    string            `json:"handle,omitempty"`
	CreatedAt time.Time         `json:"-"`
	CreatedBy *Account          `json:"-"`
	UpdatedAt time.Time         `json:"-"`
	Flags     FlagBits          `json:"flags,omitempty"`
	Metadata  *AccountMetadata  `json:"metadata,omitempty"`
	Pub       vocab.Item        `json:"-"`
	Followers AccountCollection `json:"-"`
	Following AccountCollection `json:"-"`
	Blocked   AccountCollection `json:"-"`
	Ignored   AccountCollection `json:"-"`
	Level     uint8             `json:"-"`
	Parent    Renderable        `json:"-"`
	children  RenderableList
}

var ValidActorTypes = vocab.ActivityVocabularyTypes{
	vocab.PersonType,
	vocab.ServiceType,
	vocab.GroupType,
	vocab.ApplicationType,
	vocab.OrganizationType,
}

func (a *Account) ID() Hash {
	if a == nil {
		return AnonymousHash
	}
	return a.Hash
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
	if a == nil {
		return false
	}
	return a.IsLocal() && (a.Hash.IsValid() || a.Handle == selfName) || a.IsFederated() && a.Pub != nil
}

// AP returns the underlying actvitypub item
func (a *Account) AP() vocab.Item {
	if a == nil {
		return nil
	}
	return a.Pub
}

// Private
func (a *Account) Private() bool {
	return a != nil && a.Flags&FlagsPrivate == FlagsPrivate
}

// IsModerator
func (a *Account) IsModerator() bool {
	return a != nil && (a.Flags&FlagsModerator == FlagsModerator || a.IsOperator())
}

// IsOperator
func (a *Account) IsOperator() bool {
	return a != nil && a.Flags&FlagsOperator == FlagsOperator
}

// IsApplication
func (a *Account) IsApplication() bool {
	return a != nil && a.Flags&FlagsApplication == FlagsApplication
}

// IsGroup
func (a *Account) IsGroup() bool {
	return a != nil && a.Flags&FlagsGroup == FlagsGroup
}

// IsService
func (a *Account) IsService() bool {
	return a != nil && a.Flags&FlagsService == FlagsService
}

func (a Account) Votes() VoteCollection {
	if !a.IsLogged() {
		return nil
	}
	votes := make(VoteCollection, 0)
	for _, it := range a.Metadata.Outbox {
		if !ValidAppreciationTypes.Contains(it.GetType()) {
			continue
		}
		v := Vote{}
		if err := v.FromActivityPub(it); err != nil {
			continue
		}
		if v.Item == nil {
			continue
		}
		if !votes.Contains(v) {
			votes = append(votes, v)
		}
	}
	return votes
}

// Deletable
type Deletable interface {
	Deleted() bool
	Delete()
	UnDelete()
}

func (a Account) VotedOn(i Item) *Vote {
	allVotes := make(VoteCollection, 0)
	for _, v := range a.Votes() {
		if v.Item == nil {
			continue
		}
		if itemsEqual(*v.Item, i) {
			allVotes = append(allVotes, v)
		}
	}
	sort.Slice(allVotes, func(i, j int) bool {
		return allVotes[i].SubmittedAt.Sub(allVotes[j].SubmittedAt) > 0
	})
	if len(allVotes) == 0 {
		return nil
	}
	return &allVotes[0]
}

func (a Account) GetLink() string {
	if a.IsLocal() {
		return fmt.Sprintf("/~%s", a.Handle)
	}
	return a.Metadata.URL
}

// IsLogged should show if current user was loaded from a session
func (a *Account) IsLogged() bool {
	return a != nil && (!a.CreatedAt.IsZero() || (a.Hash != AnonymousHash && a.Handle != Anonymous))
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

func (a *Account) Children() *RenderableList {
	return &a.children
}

// First
func (a AccountCollection) First() (*Account, error) {
	for _, act := range a {
		return &act, nil
	}
	return nil, errors.Errorf("empty %T", a)
}

func (a AccountCollection) Contains(b Account) bool {
	for _, acc := range a {
		if acc.Hash == b.Hash {
			return true
		}
	}
	return false
}

func (a AccountCollection) Split(pieceCount int) []AccountCollection {
	l := len(a)
	if l <= pieceCount {
		return []AccountCollection{a}
	}
	ret := make([]AccountCollection, 0)
	for i := 0; i <= l/pieceCount; i++ {
		st := i * pieceCount
		if st > l {
			break
		}
		end := (i + 1) * pieceCount
		if end > l {
			end = l
		}
		ret = append(ret, a[st:end])
	}
	return ret
}

func (h *handler) accountFromPost(r *http.Request) (Account, error) {
	if r.Method != http.MethodPost {
		return AnonymousAccount, errors.Errorf("invalid http method type")
	}

	a := &AnonymousAccount
	hash := r.PostFormValue("hash")
	if len(hash) > 0 {
		// NOTE(marius): coming from an invite
		s := h.storage
		a, _ = s.LoadAccount(r.Context(), actors.IRI(s.BaseURL()).AddPath(hash))
	}
	if accountsEqual(*a, AnonymousAccount) {
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
	handle := strings.TrimSpace(r.PostFormValue("handle"))
	if len(handle) > 0 {
		a.Handle = handle
	}
	a.Metadata = &AccountMetadata{
		Password: []byte(pw),
	}
	return *a, nil
}

func accountsFromRequestHandle(r *http.Request) (AccountCollection, error) {
	handle := chi.URLParam(r, "handle")
	if handle == "" {
		return nil, errors.NotFoundf("missing account handle %s", handle)
	}
	repo := ContextRepository(r.Context())

	return repo.accounts(r.Context(), FilterAccountByHandle(handle))
}

type AccountPtrCollection []*Account

func reparentAccounts(allAccounts *AccountPtrCollection) {
	if len(*allAccounts) == 0 {
		return
	}
	parFn := func(t AccountPtrCollection, cur *Account) *Account {
		for _, n := range t {
			if cur.CreatedBy.IsValid() {
				if cur.CreatedBy.ID() == n.ID() {
					return n
				}
			}
		}
		return nil
	}

	retAccounts := make(AccountPtrCollection, 0)
	for _, cur := range *allAccounts {
		if par := parFn(*allAccounts, cur); par != nil {
			par.children.Append(cur)
			cur.Parent = par
		} else {
			retAccounts = append(retAccounts, cur)
		}
	}
	*allAccounts = retAccounts
}
