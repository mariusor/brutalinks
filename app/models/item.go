package models

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/juju/errors"
)

type ItemMetadata []byte

type Identifiable interface {
	Id() int64
}

type Item struct {
	Hash        Hash         `json:"key"`
	Title       string       `json:"title"`
	MimeType    string       `json:"mimeType"`
	Data        string       `json:"data"`
	Score       int64        `json:"score"`
	SubmittedAt time.Time    `json:"createdAt"`
	SubmittedBy *Account     `json:"submittedBy"`
	UpdatedAt   time.Time    `json:"-"`
	Flags       FlagBits     `json:"-"`
	Path        []byte       `json:"-"`
	FullPath    []byte       `json:"-"`
	Metadata    ItemMetadata `json:"-"`
	IsTop       bool         `json:"isTop"`
	Parent      *Item        `json:"-"`
	OP          *Item        `json:"-"`
}

func (i Item) Deleted() bool {
	return (i.Flags & FlagsDeleted) == FlagsDeleted
}

func (i Item) UnDelete() {
	i.Flags ^= FlagsDeleted
}
func (i *Item) Delete() {
	i.Flags &= FlagsDeleted
}
func (i Item) IsLink() bool {
	return i.MimeType == MimeTypeURL
}

func (i Item) GetDomain() string {
	if !i.IsLink() {
		return ""
	}
	return strings.Split(i.Data, "/")[2]
}

func (i Item) ISODate() string {
	return i.SubmittedAt.Format("2006-01-02T15:04:05.000-07:00")
}

func (i Item) FromNow() string {
	td := time.Now().Sub(i.SubmittedAt)
	pluralize := func(d float64, unit string) string {
		if math.Round(d) != 1 {
			if unit == "century" {
				unit = "centurie"
			}
			return unit + "s"
		}
		return unit
	}
	val := 0.0
	unit := ""
	when := "ago"

	hours := math.Abs(td.Hours())
	minutes := math.Abs(td.Minutes())
	seconds := math.Abs(td.Seconds())

	if td.Seconds() < 0 {
		// we're in the future
		when = "in the future"
	}
	if seconds < 30 {
		return "now"
	}
	if hours < 1 {
		if minutes < 1 {
			val = math.Mod(seconds, 60)
			unit = "second"
		} else {
			val = math.Mod(minutes, 60)
			unit = "minute"
		}
	} else if hours < 24 {
		val = hours
		unit = "hour"
	} else if hours < 168 {
		val = hours / 24
		unit = "day"
	} else if hours < 672 {
		val = hours / 168
		unit = "week"
	} else if hours < 8760 {
		val = hours / 672
		unit = "month"
	} else if hours < 87600 {
		val = hours / 8760
		unit = "year"
	} else if hours < 876000 {
		val = hours / 87600
		unit = "decade"
	} else {
		val = hours / 876000
		unit = "century"
	}
	return fmt.Sprintf("%.0f %s %s", val, pluralize(val, unit), when)
}

func (i Item) IsSelf() bool {
	mimeComponents := strings.Split(i.MimeType, "/")
	return mimeComponents[0] == "text"
}

func (i ItemCollection) First() (*Item, error) {
	for _, it := range i {
		return &it, nil
	}
	return nil, errors.Errorf("empty %T", i)
}
