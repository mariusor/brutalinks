package app

import (
	"net/url"
	"strings"
	"time"

	"github.com/juju/errors"
)

const (
	ScoreMultiplier = 1
	ScoreMaxK       = 10000.0
	ScoreMaxM       = 10000000.0
	ScoreMaxB       = 10000000000.0
)

type VoteCollection []Vote

type Vote struct {
	SubmittedBy *Account  `json:"-"`
	SubmittedAt time.Time `json:"-"`
	UpdatedAt   time.Time `json:"-"`
	Weight      int       `json:"weight"`
	Item        *Item     `json:"on"`
	Flags       FlagBits  `json:"-"`
}

func trimHash(s Hash) Hash {
	h, err := url.PathUnescape(string(s))
	if err != nil {
		return ""
	}
	h = strings.TrimSpace(h)
	if len(h) == 0 {
		return ""
	}
	return Hash(h)
}

type ScoreType int

const (
	ScoreItem = ScoreType(iota)
	ScoreAccount
)

type Score struct {
	ID          int64
	Max         int64
	Ups         int64
	Downs       int64
	Key         Key
	Score       int64
	SubmittedAt time.Time
	Type        ScoreType
}

func (v VoteCollection) First() (*Vote, error) {
	for _, vv := range v {
		return &vv, nil
	}
	return nil, errors.Errorf("empty %T", v)
}
