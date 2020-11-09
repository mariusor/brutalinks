package app

import (
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"html/template"
	"strings"
	"time"
)

var ValidModerationActivityTypes = pub.ActivityVocabularyTypes{pub.BlockType, pub.IgnoreType, pub.FlagType}

// ModerationRequests
type ModerationRequests []ModerationOp

type Moderatable interface {
	IsDelete() bool
	IsUpdate() bool
	IsBlock() bool
	IsReport() bool
	IsIgnore() bool
}

type ModerationGroup struct {
	Hash        Hash            `json:"hash"`
	Icon        template.HTML   `json:"-"`
	SubmittedAt time.Time       `json:"-"`
	Object      Renderable      `json:"-"`
	Requests    []*ModerationOp `json:"-"`
	Followup    []*ModerationOp `json:"-"`
}

func (m ModerationGroup) ID() Hash {
	return m.Hash
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
	return m.Object.AP()
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

// IsDelete returns true if current moderation request is a delete
func (m ModerationGroup) IsDelete() bool {
	if len(m.Requests) == 0 {
		return false
	}
	return m.Requests[0].IsDelete()
}

// IsUpdate returns true if current moderation request is an update
func (m ModerationGroup) IsUpdate() bool {
	if len(m.Requests) == 0 {
		return false
	}
	return m.Requests[0].IsUpdate()
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

type ModerationOp struct {
	Hash        Hash                `json:"hash"`
	Icon        template.HTML       `json:"-"`
	SubmittedAt time.Time           `json:"-"`
	Data        string              `json:"-"`
	MimeType    string              `json:"-"`
	SubmittedBy *Account            `json:"by,omitempty"`
	Object      Renderable          `json:"-"`
	Metadata    *ModerationMetadata `json:"-"`
	pub         pub.Item            `json:"-"`
	Flags       FlagBits            `json:"flags,omitempty"`
}

type ModerationMetadata struct {
	ID        string        `json:"-"`
	InReplyTo pub.IRIs      `json:"-"`
	Tags      TagCollection `json:"tags,omitempty"`
	Mentions  TagCollection `json:"mentions,omitempty"`
}

func (m ModerationOp) ID() Hash {
	return m.Hash
}

// Type
func (m ModerationOp) Type() RenderType {
	return ModerationType
}

// IsValid returns if the current follow request has a hash with length greater than 0
func (m *ModerationOp) IsValid() bool {
	return m != nil && m.Hash.IsValid()
}

// IsBlock returns true if current moderation request is a block
func (m ModerationOp) IsBlock() bool {
	if m.pub == nil {
		return false
	}
	return m.pub.GetType() == pub.BlockType
}

// IsBlock returns true if current moderation request is a delete
func (m ModerationOp) IsDelete() bool {
	if m.pub == nil {
		return false
	}
	return m.pub.GetType() == pub.DeleteType
}

// IsUpdate returns true if current moderation request is an update
func (m ModerationOp) IsUpdate() bool {
	if m.pub == nil {
		return false
	}
	return m.pub.GetType() == pub.UpdateType
}

// IsIgnore returns true if current moderation request is a ignore
func (m ModerationOp) IsIgnore() bool {
	if m.pub == nil {
		return false
	}
	return m.pub.GetType() == pub.IgnoreType
}

// IsReport returns true if current moderation request is a report
func (m ModerationOp) IsReport() bool {
	if m.pub == nil {
		return false
	}
	return m.pub.GetType() == pub.FlagType
}

// AP returns the underlying actvitypub item
func (m *ModerationOp) AP() pub.Item {
	return m.pub
}

// Content returns the reason for it
func (m ModerationOp) Content() map[string][]byte {
	return map[string][]byte{m.MimeType: []byte(m.Data)}
}

// Tags returns the tags associated with the current ModerationOp
func (m ModerationOp) Tags() TagCollection {
	return m.Metadata.Tags
}

// Mentions returns the mentions associated with the current ModerationOp
func (m ModerationOp) Mentions() TagCollection {
	return m.Metadata.Mentions
}

// Date
func (m ModerationOp) Date() time.Time {
	return m.SubmittedAt
}

// Private
func (m *ModerationOp) Private() bool {
	return m.Flags&FlagsPrivate == FlagsPrivate
}

// Deleted
func (m *ModerationOp) Deleted() bool {
	return m.Flags&FlagsDeleted == FlagsDeleted
}

func (m *ModerationOp) FromActivityPub(it pub.Item) error {
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
		m.Metadata = &ModerationMetadata{
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
			} else {
				m.Object = &DeletedAccount
			}
		}
		if strings.Contains(a.Object.GetLink().String(), "objects") {
			ob := new(Item)
			if err := ob.FromActivityPub(a.Object); err == nil {
				m.Object = ob
			} else {
				m.Object = &DeletedItem
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
		m.Metadata = &ModerationMetadata{
			ID: string(a.ID),
		}
		if a.InReplyTo != nil {
			m.Metadata.InReplyTo = make(pub.IRIs, 0)
			pub.OnCollectionIntf(a.InReplyTo, func(col pub.CollectionInterface) error {
				for _, it := range col.Collection() {
					m.Metadata.InReplyTo = append(m.Metadata.InReplyTo, it.GetLink())
				}
				return nil
			})
		}
		if a.Tag != nil && len(a.Tag) > 0 {
			m.Metadata.Tags = make(TagCollection, 0)
			m.Metadata.Mentions = make(TagCollection, 0)

			tags := TagCollection{}
			tags.FromActivityPub(a.Tag)
			for _, t := range tags {
				if t.Type == TagTag {
					m.Metadata.Tags = append(m.Metadata.Tags, t)
				}
				if t.Type == TagMention {
					m.Metadata.Mentions = append(m.Metadata.Mentions, t)
				}
			}
		}

		return nil
	})
}

func (m *ModerationGroup) FromActivityPub(it pub.Item) error {
	op := new(ModerationOp)
	if err := op.FromActivityPub(it); err != nil {
		return err
	}
	m.Object = op
	return nil
}

func moderationGroupAtIndex(groups []*ModerationGroup, r ModerationOp) int {
	for i, g := range groups {
		if g.Object == nil || r.Object == nil {
			continue
		}
		gAP := g.Object.AP()
		rAP := r.Object.AP()
		if gAP.GetLink().Equals(rAP.GetLink(), false) && gAP.GetType() == rAP.GetType() {
			return i
		}
	}
	return -1
}

func moderationGroupFromRequest(r *ModerationOp) *ModerationGroup {
	mg := new(ModerationGroup)
	mg.Object = r.Object
	mg.Hash = r.Hash
	mg.SubmittedAt = r.SubmittedAt
	mg.Icon = r.Icon
	mg.Requests = make([]*ModerationOp, 1)
	mg.Requests[0] = r
	return mg
}

func aggregateModeration(rl RenderableList, followups []ModerationOp) RenderableList {
	groups := make([]*ModerationGroup, 0)
	for k, r := range rl {
		m, ok := r.(*ModerationOp)
		if !ok {
			continue
		}
		if m.Object == nil {
			continue
		}
		var mg *ModerationGroup
		if i := moderationGroupAtIndex(groups, *m); i < 0 {
			mg = moderationGroupFromRequest(m)
			groups = append(groups, mg)
		} else {
			mg = groups[i]
			mg.Requests = append(mg.Requests, m)
		}
		for _, fw := range followups {
			if mg.Object == nil || fw.Object == nil {
				continue
			}
			mObIRI := mg.Object.AP()
			fwIRI := fw.Object.AP()
			if mObIRI != nil && fwIRI != nil && mObIRI.GetLink().Equals(fwIRI.GetLink(), false) {
				mg.Followup = append(mg.Followup, &fw)
			}
		}
		rl[k] = mg
	}

	return rl
}

func (m ModerationRequests) Contains(mop ModerationOp) bool {
	for _, vv := range m {
		if vv.Metadata.ID == mop.Metadata.ID {
			return true
		}
	}
	return false
}
