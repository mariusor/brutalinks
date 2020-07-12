package app

import (
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"time"
)

// FollowRequests
type FollowRequests []FollowRequest

// FollowRequest
type FollowRequest struct {
	Hash        Hash              `json:"hash"`
	SubmittedAt time.Time         `json:"-"`
	SubmittedBy *Account          `json:"by,omitempty"`
	Object      *Account          `json:"-"`
	Metadata    *ActivityMetadata `json:"-"`
	pub         pub.Item          `json:"-"`
	Flags       FlagBits          `json:"flags,omitempty"`
}

// ActivityMetadata
type ActivityMetadata struct {
	ID string `json:"-"`
}

// FromActivityPub
func (f *FollowRequest) FromActivityPub(it pub.Item) error {
	if f == nil {
		return nil
	}
	if it == nil {
		return errors.Newf("nil item received")
	}
	f.pub = it
	if it.IsLink() {
		iri := it.GetLink()
		f.Hash.FromActivityPub(iri)
		f.Metadata = &ActivityMetadata{
			ID: iri.String(),
		}
		return nil
	}
	return pub.OnActivity(it, func(a *pub.Activity) error {
		f.Hash.FromActivityPub(a)
		wer := Account{}
		wer.FromActivityPub(a.Actor)
		f.SubmittedBy = &wer
		wed := Account{}
		wed.FromActivityPub(a.Object)
		f.Object = &wed
		f.SubmittedAt = a.Published
		f.Metadata = &ActivityMetadata{
			ID: string(a.ID),
		}

		return nil
	})
}

// Type
func (f *FollowRequest) Type() RenderType {
	return FollowType
}

// Date
func (f FollowRequest) Date() time.Time {
	return f.SubmittedAt
}

// Private
func (f *FollowRequest) Private() bool {
	return f.Flags & FlagsPrivate == FlagsPrivate
}

// Deleted
func (f *FollowRequest) Deleted() bool {
	return f.Flags & FlagsDeleted == FlagsDeleted
}

// IsValid returns if the current follow request has a hash with length greater than 0
func (f *FollowRequest) IsValid() bool {
	return f != nil && len(f.Hash) > 0
}

// AP returns the underlying actvitypub item
func (f *FollowRequest) AP() pub.Item {
	return f.pub
}
