package app

import (
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"time"
)

type FollowRequest struct {
	Hash        Hash      `json:"hash"`
	SubmittedAt time.Time `json:"-"`
	SubmittedBy *Account  `json:"-"`
	Object      *Account  `json:"-"`
	Flags       FlagBits  `json:"-"`
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
		return nil
	})
}

