package models

import (
	"time"
	"strings"
	"fmt"
	"math"
			)

type Identifiable interface {
	Id() int64
}

type Item struct {
	id          int64
	Hash        string    `json:"key"`
	Title       string    `json:"title"`
	MimeType    string    `json:"mime_type"`
	Data        string    `json:"data"`
	Score       int64     `json:"score"`
	SubmittedAt time.Time `json:"created_at"`
	SubmittedBy *Account  `json:"submitted_by"`
	UpdatedAt   time.Time `json:"updated_at"`
	Flags       int8
	Path        []byte
	FullPath    []byte
	Metadata    []byte
	IsTop       bool
	Parent		*Item
	OP		*Item
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

func LoadItem(h string) (Item, error) {
	p := item{}
	i := Item{}
	sel := `select 
			"content_items"."id", "content_items"."key", "content_items"."mime_type", "content_items"."data", 
			"content_items"."title", "content_items"."score", "content_items"."submitted_at", 
			"content_items"."submitted_by", "content_items"."flags", "content_items"."metadata", "content_items"."path",
			"accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score", 
			"accounts"."created_at", "accounts"."metadata", "accounts"."flags"
		from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
		where "content_items"."key" ~* $1`
	rows, err := Db.Query(sel, h)
	if err != nil {
		return i, err
	}
	for rows.Next() {
		a := account{}
		var aKey, iKey []byte
		err := rows.Scan(
			&p.Id, &iKey, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
			&a.Id, &aKey, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return i, err
		}
		p.Key.FromBytes(iKey)
		a.Key.FromBytes(aKey)
		acct := loadFromModel(a)
		p.SubmittedByAccount = &acct
	}
	i = loadItemFromContent(p)
	return i, nil
}

func loadItemFromContent(c item) Item {
	i := Item{
		Hash:        c.Hash(),
		UpdatedAt:   c.UpdatedAt,
		SubmittedAt: c.SubmittedAt,
		SubmittedBy: c.SubmittedByAccount,
		MimeType:    c.MimeType,
		Score:       int64(float64(c.Score) / ScoreMultiplier),
		Flags:       c.Flags,
		Metadata:    c.Metadata,
		Path:        c.Path,
		FullPath:    c.FullPath(),
		IsTop:       c.IsTop(),
	}
	if len(c.Title) > 0 {
		i.Title = string(c.Title)
	}
	if len(c.Data) > 0 {
		i.Data = string(c.Data)
	}
	parentHash := c.GetParentHash()
	if len(parentHash) > 0 {
		i.Parent= 	 &Item{
			Hash: parentHash,
		}
	}
	opHash := c.GetOPHash()
	if len(opHash) > 0 && opHash != parentHash {
		i.OP = &Item{
			Hash: opHash,
		}
	}
	return i
}
