package brutalinks

import (
	"html/template"
	"strings"
	"time"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
)

var ValidModerationActivityTypes = vocab.ActivityVocabularyTypes{vocab.BlockType, vocab.IgnoreType, vocab.FlagType}

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

func (m *ModerationGroup) ID() Hash {
	if m == nil {
		return AnonymousHash
	}
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
func (m *ModerationGroup) AP() vocab.Item {
	return m.Object.AP()
}

// Date
func (m ModerationGroup) Date() time.Time {
	return m.Requests[0].SubmittedAt
}

func (m *ModerationGroup) Children() *RenderableList {
	return nil
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
	Pub         vocab.Item          `json:"-"`
	Flags       FlagBits            `json:"flags,omitempty"`
}

type ModerationMetadata struct {
	ID        string        `json:"-"`
	InReplyTo vocab.IRIs    `json:"-"`
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
	if m.Pub == nil {
		return false
	}
	return m.Pub.GetType() == vocab.BlockType
}

// IsBlock returns true if current moderation request is a delete
func (m ModerationOp) IsDelete() bool {
	if m.Pub == nil {
		return false
	}
	return m.Pub.GetType() == vocab.DeleteType
}

// IsUpdate returns true if current moderation request is an update
func (m ModerationOp) IsUpdate() bool {
	if m.Pub == nil {
		return false
	}
	return m.Pub.GetType() == vocab.UpdateType
}

// IsIgnore returns true if current moderation request is a ignore
func (m ModerationOp) IsIgnore() bool {
	if m.Pub == nil {
		return false
	}
	return m.Pub.GetType() == vocab.IgnoreType
}

// IsReport returns true if current moderation request is a report
func (m ModerationOp) IsReport() bool {
	if m.Pub == nil {
		return false
	}
	return m.Pub.GetType() == vocab.FlagType
}

// AP returns the underlying actvitypub item
func (m *ModerationOp) AP() vocab.Item {
	return m.Pub
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

func (m *ModerationOp) Children() *RenderableList {
	return nil
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

func GetRenderableByType(typ vocab.ActivityVocabularyType) Renderable {
	var result Renderable
	if ValidAppreciationTypes.Contains(typ) {
		result = new(Vote)
	}
	if ValidModerationActivityTypes.Contains(typ) {
		result = new(ModerationOp)
	}
	if ValidActorTypes.Contains(typ) {
		result = new(Account)
	}
	if ValidContentTypes.Contains(typ) {
		result = new(Item)
	}
	return result
}

func loadItemActorOrActivityFromModerationActivityObject(it vocab.Item) Renderable {
	result, _ := LoadFromActivityPubItem(it)
	return result
}

func (m *ModerationOp) FromActivityPub(it vocab.Item) error {
	if m == nil {
		return nil
	}
	if vocab.IsNil(it) {
		return errors.Newf("nil item received")
	}
	m.Pub = it
	if it.IsLink() {
		iri := it.GetLink()
		m.Hash.FromActivityPub(iri)
		m.Metadata = &ModerationMetadata{
			ID: iri.String(),
		}
		return nil
	}
	return vocab.OnActivity(it, func(a *vocab.Activity) error {
		m.Hash.FromActivityPub(a)
		wer := new(Account)

		m.Icon = icon(strings.ToLower(string(a.Type)))
		wer.FromActivityPub(a.Actor)
		m.SubmittedBy = wer
		if a.Object.IsCollection() {
			cc := make([]Renderable, 0)
			vocab.OnCollectionIntf(a.Object, func(c vocab.CollectionInterface) error {
				for _, it := range c.Collection() {
					cc = append(cc, loadItemActorOrActivityFromModerationActivityObject(it))
				}
				return nil
			})
		} else {
			m.Object = loadItemActorOrActivityFromModerationActivityObject(a.Object)
		}
		reason := new(Item)
		vocab.OnObject(a, func(o *vocab.Object) error {
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
			m.Metadata.InReplyTo = make(vocab.IRIs, 0)
			vocab.OnCollectionIntf(a.InReplyTo, func(col vocab.CollectionInterface) error {
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

func (m *ModerationGroup) FromActivityPub(it vocab.Item) error {
	op := new(ModerationOp)
	if err := op.FromActivityPub(it); err != nil {
		return err
	}
	m.Object = op
	return nil
}

func moderationGroupAtIndex(groups []*ModerationGroup, r ModerationOp) int {
	for i, g := range groups {
		if g == nil || g.AP() == nil {
			continue
		}
		if len(g.Requests) > 0 {
			req := g.Requests[0]
			if req.AP().GetType() != r.AP().GetType() || req.Hash == r.Hash {
				continue
			}
		}
		if g.Object == nil || r.Object == nil {
			continue
		}
		gAP := g.Object.AP()
		rAP := r.Object.AP()
		if gAP.GetType() == rAP.GetType() && gAP.GetLink().Equals(rAP.GetLink(), false) {
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
	result := make(RenderableList, 0)
	for _, r := range rl {
		m, ok := r.(*ModerationOp)
		if !ok {
			result = append(result, r)
			continue
		}
		if m.Object == nil {
			continue
		}
		var mg *ModerationGroup
		if i := moderationGroupAtIndex(groups, *m); i < 0 {
			mg = moderationGroupFromRequest(m)
			groups = append(groups, mg)
			result = append(result, mg)
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
	}

	return result
}

func (m ModerationRequests) Contains(mop ModerationOp) bool {
	for _, vv := range m {
		if vv.Metadata.ID == mop.Metadata.ID {
			return true
		}
	}
	return false
}
