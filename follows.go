package brutalinks

import (
	"time"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
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
	pub         vocab.Item        `json:"-"`
	Flags       FlagBits          `json:"flags,omitempty"`
}

// ActivityMetadata
type ActivityMetadata struct {
	ID        string     `json:"-"`
	InReplyTo vocab.IRIs `json:"-"`
}

func (f *FollowRequest) ID() Hash {
	if f == nil {
		return AnonymousHash
	}
	return f.Hash
}

// FromActivityPub
func (f *FollowRequest) FromActivityPub(it vocab.Item) error {
	if f == nil {
		return nil
	}
	if vocab.IsNil(it) {
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
	return vocab.OnActivity(it, func(a *vocab.Activity) error {
		err := f.Hash.FromActivityPub(a)
		if err != nil {
			return err
		}
		wer := new(Account)
		err = wer.FromActivityPub(a.Actor)
		if err != nil {
			return err
		}
		f.SubmittedBy = wer
		wed := new(Account)
		err = wed.FromActivityPub(a.Object)
		if err != nil {
			return err
		}
		f.Object = wed
		f.SubmittedAt = a.Published
		f.Metadata = &ActivityMetadata{
			ID: string(a.ID),
		}
		if a.InReplyTo != nil {
			f.Metadata.InReplyTo = make(vocab.IRIs, 0)
			vocab.OnCollectionIntf(a.InReplyTo, func(col vocab.CollectionInterface) error {
				for _, it := range col.Collection() {
					f.Metadata.InReplyTo = append(f.Metadata.InReplyTo, it.GetLink())
				}
				return nil
			})
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

func (f *FollowRequest) Children() *RenderableList {
	return nil
}

// Private
func (f *FollowRequest) Private() bool {
	return f.Flags&FlagsPrivate == FlagsPrivate
}

// Deleted
func (f *FollowRequest) Deleted() bool {
	return f.Flags&FlagsDeleted == FlagsDeleted
}

// IsValid returns if the current follow request has a hash with length greater than 0
func (f *FollowRequest) IsValid() bool {
	return f != nil && f.Hash.IsValid()
}

// AP returns the underlying actvitypub item
func (f *FollowRequest) AP() vocab.Item {
	return f.pub
}
