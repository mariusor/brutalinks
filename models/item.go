package models

import (
	"time"
	"strings"
	"fmt"
	"math"
	"github.com/juju/errors"
	"encoding/json"
	log "github.com/sirupsen/logrus"
)

type Identifiable interface {
	Id() int64
}

type Item struct {
	Hash        string    `json:"key"`
	Title       string    `json:"title"`
	MimeType    string    `json:"mimeType"`
	Data        string    `json:"data"`
	Score       int64     `json:"score"`
	SubmittedAt time.Time `json:"createdAt"`
	SubmittedBy *Account  `json:"submittedBy"`
	UpdatedAt   time.Time `json:"-"`
	Flags       int8      `json:"-"`
	Path        []byte    `json:"-"`
	FullPath    []byte    `json:"-"`
	Metadata    []byte    `json:"-"`
	IsTop       bool      `json:"isTop"`
	Parent		*Item     `json:"-"`
	OP		*Item         `json:"-"`
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

func loadItemFromModel(c item) Item {
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

func SaveItem(it Item) (Item, error) {
	i := item{
		Flags: it.Flags,
		Score: it.Score,
		MimeType: it.MimeType,
		Data: []byte(it.Data),
		Title: []byte(it.Title),
	}

	if it.Metadata != nil{
		jMetadata, err := json.Marshal(it.Metadata)
		log.Warning(err)
		i.Metadata = jMetadata
	}
	i.GetKey()

	var params = make([]interface{}, 0)
	params = append(params, i.Key.Bytes())
	params = append(params, i.Title)
	params = append(params, i.Data)
	params = append(params, i.MimeType)
	params = append(params, it.SubmittedBy.Hash)

	var ins string
	if it.Parent != nil {
		ins = `insert into "content_items" ("key", "title", "data", "mime_type", "submitted_by", "path") 
		values(
			$1, $2, $3, $4, (select "id" from "accounts" where "key" ~* $5), (select (case when "path" is not null then concat("path", '.', "key") else "key" end) 
				as "parent_path" from "content_items" where key ~* $6)::ltree
		)`
		params = append(params, it.Parent.Hash)
	} else {
		ins = `insert into "content_items" ("key", "title", "data", "mime_type", "submitted_by") 
		values($1, $2, $3, $4, (select "id" from "accounts" where "key" ~* $5))`
	}

	res, err := Db.Exec(ins, params...)
	if err != nil {
		return Item{}, err
	} else {
		if rows, _ := res.RowsAffected(); rows == 0 {
			return Item{}, errors.Errorf("could not save item %q", i.Hash)
		}
	}

	return LoadItem(LoadItemFilter{Key: i.Key.String()})
}
