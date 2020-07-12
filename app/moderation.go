package app

import (
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"html/template"
	"strings"
	"time"
)

var ModerationActivityTypes = pub.ActivityVocabularyTypes{pub.BlockType, pub.IgnoreType, pub.FlagType}

// ModerationRequests
type ModerationRequests []ModerationRequest

type Moderatable interface {
	IsBlock() bool
	IsReport() bool
	IsIgnore() bool
}

type ModerationGroup struct {
	Hash        Hash                 `json:"hash"`
	Icon        template.HTML        `json:"-"`
	SubmittedAt time.Time            `json:"-"`
	Object      Renderable           `json:"-"`
	Requests    []*ModerationRequest `json:"-"`
}

// Hash
func (m *ModerationGroup) Private() bool {
	return false
}

// Hash
func (m *ModerationGroup) Deleted() bool {
	return false
}

// Type
func (m *ModerationGroup) Type() RenderType {
	return ModerationType
}

// IsValid returns if the current follow request group has a hash with length greater than 0
func (m *ModerationGroup) IsValid() bool {
	return m != nil && m.Object.IsValid() && len(m.Requests) > 0
}

// AP returns the underlying actvitypub item
func (m *ModerationGroup) AP() pub.Item {
	return nil
}

// Date
func (m ModerationGroup) Date() time.Time {
	return m.Requests[0].SubmittedAt
}

// IsBlock returns true if current moderation request is a block
func (m ModerationGroup) IsBlock() bool {
	if len(m.Requests) == 0 {
		return false
	}
	return m.Requests[0].IsBlock()
}

// IsIgnore returns true if current moderation request is a ignore
func (m ModerationGroup) IsIgnore() bool {
	if len(m.Requests) == 0 {
		return false
	}
	return m.Requests[0].IsIgnore()
}

// IsReport returns true if current moderation request is a report
func (m ModerationGroup) IsReport() bool {
	if len(m.Requests) == 0 {
		return false
	}
	return m.Requests[0].IsReport()
}

type ModerationRequest struct {
	Hash        Hash              `json:"hash"`
	Icon        template.HTML     `json:"-"`
	SubmittedAt time.Time         `json:"-"`
	Data        string            `json:"-"`
	MimeType    string            `json:"-"`
	SubmittedBy *Account          `json:"by,omitempty"`
	Object      Renderable        `json:"-"`
	Metadata    *ActivityMetadata `json:"-"`
	pub         pub.Item          `json:"-"`
	Flags       FlagBits          `json:"flags,omitempty"`
}

// Type
func (m *ModerationRequest) Type() RenderType {
	return ModerationType
}

// IsValid returns if the current follow request has a hash with length greater than 0
func (m *ModerationRequest) IsValid() bool {
	return m != nil && len(m.Hash) > 0
}

// IsBlock returns true if current moderation request is a block
func (m ModerationRequest) IsBlock() bool {
	if m.pub == nil {
		return false
	}
	return m.pub.GetType() == pub.BlockType
}

// IsIgnore returns true if current moderation request is a ignore
func (m ModerationRequest) IsIgnore() bool {
	if m.pub == nil {
		return false
	}
	return m.pub.GetType() == pub.IgnoreType
}

// IsReport returns true if current moderation request is a report
func (m ModerationRequest) IsReport() bool {
	if m.pub == nil {
		return false
	}
	return m.pub.GetType() == pub.FlagType
}

// AP returns the underlying actvitypub item
func (m *ModerationRequest) AP() pub.Item {
	return m.pub
}

// Date
func (m ModerationRequest) Date() time.Time {
	return m.SubmittedAt
}

// Private
func (m *ModerationRequest) Private() bool {
	return m.Flags&FlagsPrivate == FlagsPrivate
}

// Deleted
func (m *ModerationRequest) Deleted() bool {
	return m.Flags&FlagsDeleted == FlagsDeleted
}

func (m *ModerationRequest) FromActivityPub(it pub.Item) error {
	if m == nil {
		return nil
	}
	if it == nil {
		return errors.Newf("nil item received")
	}
	m.pub = it
	if it.IsLink() {
		iri := it.GetLink()
		m.Hash.FromActivityPub(iri)
		m.Metadata = &ActivityMetadata{
			ID: iri.String(),
		}
		return nil
	}
	return pub.OnActivity(it, func(a *pub.Activity) error {
		m.Hash.FromActivityPub(a)
		wer := new(Account)

		m.Icon = icon(strings.ToLower(string(a.Type)))
		wer.FromActivityPub(a.Actor)
		m.SubmittedBy = wer
		if strings.Contains(a.Object.GetLink().String(), "actors") {
			ob := new(Account)
			if err := ob.FromActivityPub(a.Object); err == nil {
				m.Object = ob
			}
		}
		if strings.Contains(a.Object.GetLink().String(), "objects") {
			ob := new(Item)
			if err := ob.FromActivityPub(a.Object); err == nil {
				m.Object = ob
			}
		}
		reason := new(Item)
		pub.OnObject(a, func(o *pub.Object) error {
			return FromArticle(reason, o)
		})
		if len(reason.Data) > 0 {
			m.Data = reason.Data
		}
		if len(reason.MimeType) > 0 {
			m.MimeType = reason.MimeType
		}
		m.SubmittedAt = a.Published
		m.Metadata = &ActivityMetadata{
			ID: string(a.ID),
		}

		return nil
	})
}

func moderationGroupAtIndex(groups []*ModerationGroup, r ModerationRequest) int {
	for i, g := range groups {
		gAP := g.Object.AP()
		rAP := r.Object.AP()
		if gAP.GetLink().Equals(rAP.GetLink(), false) && gAP.GetType() == rAP.GetType() {
			return i
		}
	}
	return -1
}

func moderationGroupFromRequest(r *ModerationRequest) *ModerationGroup {
	mg := new(ModerationGroup)
	mg.Object = r.Object
	mg.Hash = r.Hash
	mg.SubmittedAt = r.SubmittedAt
	mg.Icon = r.Icon
	mg.Requests = make([]*ModerationRequest, 1)
	mg.Requests[0] = r
	return mg
}

func aggregateModeration(rl ...Renderable) RenderableList {
	groups := make([]*ModerationGroup, 0)
	for _, r := range rl {
		m, ok := r.(*ModerationRequest)
		if !ok {
			continue
		}
		if i := moderationGroupAtIndex(groups, *m); i < 0 {
			groups = append(groups, moderationGroupFromRequest(m))
		} else {
			mg := groups[i]
			mg.Requests = append(mg.Requests, m)
		}
	}

	result := make(RenderableList, len(groups))
	for i, m := range groups {
		result[i] = m
	}
	return result
}
