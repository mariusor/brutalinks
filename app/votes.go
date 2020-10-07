package app

import (
	pub "github.com/go-ap/activitypub"
	"net/url"
	"strings"
	"time"

	"github.com/go-ap/errors"
)

const (
	ScoreMultiplier = 1
	ScoreMaxK       = 10000.0
	ScoreMaxM       = 10000000.0
	ScoreMaxB       = 10000000000.0
)

var ValidAppreciationTypes = pub.ActivityVocabularyTypes{
	pub.LikeType,
	pub.DislikeType,
	pub.UndoType,
}

type VoteCollection []Vote

type VoteMetadata struct {
	IRI         string `json:"-"`
	OriginalIRI string `json:"-"`
}

type Vote struct {
	SubmittedBy *Account      `json:"-"`
	SubmittedAt time.Time     `json:"-"`
	UpdatedAt   time.Time     `json:"-"`
	Weight      int           `json:"weight"`
	Item        *Item         `json:"on"`
	Flags       FlagBits      `json:"-"`
	Metadata    *VoteMetadata `json:"-"`
	pub         *pub.Like     `json:"-"`
}

// HasMetadata
func (v Vote) HasMetadata() bool {
	return v.Metadata != nil
}

// IsValid
func (v *Vote) IsValid() bool {
	return v != nil && v.Item.IsValid()
}

// IsYay returns true if current vote is a Yay
func (v Vote) IsYay() bool {
	if v.pub == nil {
		return false
	}
	return v.pub.GetType() == pub.LikeType
}

// IsNay returns true if current vote is a Nay
func (v Vote) IsNay() bool {
	if v.pub == nil {
		return false
	}
	return v.pub.GetType() == pub.DislikeType
}

// AP returns the underlying actvitypub item
func (v *Vote) AP() pub.Item {
	return v.pub
}

// Type
func (v *Vote) Type() RenderType {
	return AppreciationType
}

// Date
func (v Vote) Date() time.Time {
	return v.SubmittedAt
}
func (v VoteCollection) Contains(vot Vote) bool {
	for _, vv := range v {
		if vv.Metadata.IRI == vot.Metadata.IRI {
			return true
		}
	}
	return false
}

func trimHash(s Hash) Hash {
	h, err := url.PathUnescape(string(s))
	if err != nil {
		return Hash("")
	}
	h = strings.TrimSpace(h)
	if len(h) == 0 {
		return Hash("")
	}
	return Hash(h)
}

type ScoreType int

const (
	ScoreItem = ScoreType(iota)
	ScoreAccount
)

func (v VoteCollection) First() (*Vote, error) {
	for _, vv := range v {
		return &vv, nil
	}
	return nil, errors.Errorf("empty %T", v)
}

// Score
func (v VoteCollection) Score() int {
	score := 0
	for _, vot := range v {
		score += vot.Weight
	}
	return score
}
