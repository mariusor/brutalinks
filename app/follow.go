package app

import (
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"time"
)

type FollowRequests []FollowRequest

type FollowRequest struct {
	Hash        Hash            `json:"hash"`
	SubmittedAt time.Time       `json:"-"`
	SubmittedBy *Account        `json:"-"`
	Object      *Account        `json:"-"`
	Metadata    *FollowMetadata `json:"-"`
	Flags       FlagBits        `json:"-"`
}

type FollowMetadata struct {
	ID string `json:"-"`
}

func (f *FollowRequest) FromActivityPub(it pub.Item) error {
	if f == nil {
		return nil
	}
	if it == nil {
		return errors.Newf("nil item received")
	}
	if it.IsLink() {
		iri := it.GetLink()
		f.Hash.FromActivityPub(iri)
		f.Metadata = &FollowMetadata{
			ID: iri.String(),
		}
		return nil
	}
	return pub.OnActivity(it, func(a *pub.Activity) error {
		f.Hash.FromActivityPub(a)
		follower := Account{}
		follower.FromActivityPub(a.Actor)
		f.SubmittedBy = &follower
		followed := Account{}
		followed.FromActivityPub(a.Object)
		f.Object = &followed
		f.SubmittedAt = a.Published
		f.Metadata = &FollowMetadata{
			ID: string(a.ID),
		}
		return nil
	})
}
