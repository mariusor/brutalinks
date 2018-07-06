package models

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"

	"github.com/russross/blackfriday"
)

const (
	FlagsNone    = 0
	FlagsDeleted = 1
	MimeTypeURL  = "application/url"
)

type Content struct {
	Id          int64     `orm:Id,"auto"`
	Key         []byte    `orm:key,size(56)`
	Title       []byte    `orm:title`
	MimeType    string    `orm:mime_type`
	Data        []byte    `orm:data`
	Score       int64     `orm:score`
	SubmittedAt time.Time `orm:created_at`
	SubmittedBy int64     `orm:submitted_by`
	UpdatedAt   time.Time `orm:updated_at`
	Handle      string    `orm:handle`
	Flags       int8      `orm:Flags`
	Metadata    []byte    `orm:metadata`
	Path        []byte    `orm:path`
	fullPath    []byte
	parentLink  string
}

//
//func (c Content) Id() int64 {
//	return c.Id
//}

type Item interface {
	Id() int64
}

type ContentCollection []Content

func (c Content) ParentLink() string {
	if c.parentLink == "" {
		if c.Path == nil {
			c.parentLink = "/"
		} else {
			lastDotPos := bytes.LastIndex(c.Path, []byte(".")) + 1
			parentHash := c.Path[lastDotPos : lastDotPos+8]
			c.parentLink = fmt.Sprintf("/p/%s/%s", c.Hash(), parentHash)
		}
	}
	return c.parentLink
}
func (c Content) OPLink() string {
	if c.Path != nil {
		parentHash := c.Path[0:8]
		return fmt.Sprintf("/op/%s/%s", c.Hash(), parentHash)
	}
	return "/"
}

//func (c Content) ancestorLink(lvl int) string {
//
//}
func (c Content) IsSelf() bool {
	mimeComponents := strings.Split(c.MimeType, "/")
	return mimeComponents[0] == "text"
}

func (c *Content) GetKey() []byte {
	data := c.Data
	now := c.UpdatedAt
	if now.IsZero() {
		now = time.Now()
	}
	data = append(data, []byte(fmt.Sprintf("%d", now.UnixNano()))...)
	data = append(data, []byte(c.Path)...)
	data = append(data, []byte(fmt.Sprintf("%d", c.SubmittedBy))...)

	c.Key = []byte(fmt.Sprintf("%x", sha256.Sum256(data)))
	return c.Key
}
func (c Content) scoreLink(dir string) string {
	if c.SubmittedAt.IsZero() {
		return ""
	}
	return fmt.Sprintf("/%4d/%02d/%02d/%s?%s", c.SubmittedAt.Year(), c.SubmittedAt.Month(), c.SubmittedAt.Day(), c.Key[0:8], dir)
}
func (c Content) ScoreUPLink() string {
	return c.scoreLink("yay")
}
func (c Content) ScoreDOWNLink() string {
	return c.scoreLink("nay")
}
func (c Content) IsTop() bool {
	return c.Path == nil
}
func (c Content) Hash() string {
	return c.Hash8()
}
func (c Content) Hash8() string {
	return string(c.Key[0:8])
}
func (c Content) Hash16() string {
	return string(c.Key[0:16])
}
func (c Content) Hash32() string {
	return string(c.Key[0:32])
}
func (c Content) Hash64() string {
	return string(c.Key)
}
func (c Content) PermaLink() string {
	if c.SubmittedAt.IsZero() {
		return ""
	}
	return fmt.Sprintf("/%4d/%02d/%02d/%s", c.SubmittedAt.Year(), c.SubmittedAt.Month(), c.SubmittedAt.Day(), c.Hash())
}
func (c *Content) FullPath() []byte {
	if c.fullPath == nil {
		c.fullPath = append(c.fullPath, c.Path...)
		if len(c.fullPath) > 0 {
			c.fullPath = append(c.fullPath, byte('.'))
		}
		c.fullPath = append(c.fullPath, c.Key...)
	}
	return c.fullPath
}
func (c Content) Level() int {
	if c.Path == nil {
		return 0
	}
	return bytes.Count(c.FullPath(), []byte("."))
}

func (c Content) Deleted() bool {
	return c.Flags&FlagsDeleted == FlagsDeleted
}
func (c Content) UnDelete() {
	c.Flags ^= FlagsDeleted
}
func (c *Content) Delete() {
	c.Flags &= FlagsDeleted
}
func (c Content) IsLink() bool {
	return c.MimeType == MimeTypeURL
}

func (c Content) ScoreFmt() string {
	score := 0.0
	units := ""
	base := float64(c.Score) / ScoreMultiplier
	d := math.Ceil(math.Log10(math.Abs(base)))
	if d < 5 {
		score = math.Ceil(base)
		return fmt.Sprintf("%d", int(score))
	} else if d < 8 {
		score = base / ScoreMaxK
		units = "K"
	} else if d < 11 {
		score = base / ScoreMaxM
		units = "M"
	} else if d < 13 {
		score = base / ScoreMaxB
		units = "B"
	} else {
		sign := ""
		if base < 0 {
			sign = "-"
		}
		return fmt.Sprintf("%s%s", sign, "∞")
	}

	return fmt.Sprintf("%3.1f%s", score, units)
}
func (c Content) GetDomain() string {
	if !c.IsLink() {
		return ""
	}
	return strings.Split(string(c.Data), "/")[2]
}

func (c ContentCollection) GetAllIds() []int64 {
	var i []int64
	for _, k := range c {
		i = append(i, k.Id)
	}
	return i
}

func (c Content) FromNow() string {
	i := time.Now().Sub(c.SubmittedAt)
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

	hours := math.Abs(i.Hours())
	minutes := math.Abs(i.Minutes())
	seconds := math.Abs(i.Seconds())

	if i.Seconds() < 0 {
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

func (c Content) ISODate() string {
	return c.SubmittedAt.Format("2006-01-02T15:04:05.000-07:00")
}
func (c Content) HTML() template.HTML {
	return template.HTML(string(c.Data))
}
func (c Content) Markdown() template.HTML {
	return template.HTML(blackfriday.MarkdownCommon(c.Data))
}
func (c Content) Text() string {
	return string(c.Data)
}
