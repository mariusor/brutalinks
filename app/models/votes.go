package models

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

type VoteCollection map[Hash]Vote

type Vote struct {
	SubmittedBy *Account  `json:"submittedBy"`
	SubmittedAt time.Time `json:"-"`
	UpdatedAt   time.Time `json:"-"`
	Weight      int       `json:"weight"`
	Item        *Item     `json:"-"`
	Flags       FlagBits  `json:"-"`
}

func trimHash(s string) string {
	h, err := url.PathUnescape(s)
	if err != nil {
		return ""
	}
	h = strings.TrimSpace(h)
	if len(h) == 0 {
		return ""
	}
	return h
}

type ScoreType int

const (
	ScoreItem = ScoreType(iota)
	ScoreAccount
)

type Score struct {
	ID        int64
	Key       []byte
	Score     int64
	Submitted time.Time
	Type      ScoreType
}

func (v VoteCollection) First() (*Vote, error) {
	for _, vv := range v {
		return &vv, nil
	}
	return nil, errors.Errorf("empty %T", v)
}
