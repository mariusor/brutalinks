package models

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"database/sql"

	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
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
		i.Parent = &Item{
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
	return saveItem(Config.DB, it)
}

func saveItem(db *sql.DB, it Item) (Item, error) {
	i := item{
		Flags:    it.Flags,
		Score:    it.Score,
		MimeType: it.MimeType,
		Data:     []byte(it.Data),
		Title:    []byte(it.Title),
	}

	if it.Metadata != nil {
		jMetadata, err := json.Marshal(it.Metadata)
		log.WithFields(log.Fields{}).Warning(err)
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
	if it.Parent != nil && len(it.Parent.Hash) > 0 {
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

	res, err := db.Exec(ins, params...)
	if err != nil {
		return Item{}, err
	} else {
		if rows, _ := res.RowsAffected(); rows == 0 {
			return Item{}, errors.Errorf("could not save item %q", i.Hash())
		}
	}

	return loadItem(db, LoadItemsFilter{Key: []string{i.Key.String()}})
}

func saveVote(db *sql.DB, vot Vote) (Vote, error) {
	var sel string
	sel = `select "votes"."id", "accounts"."id", "votes"."weight", "votes"."submitted_at" from "votes" inner join "accounts" on "accounts"."id" = "votes"."submitted_by" 
			where "accounts"."key" ~* $1 and "votes"."item_id" = (select "id" from "content_items" where "key" ~* $2);`
	var userId int64
	var vId int64
	var oldWeight int64
	var submittedAt time.Time
	{
		rows, err := db.Query(sel, vot.SubmittedBy.Hash, vot.Item.Hash)
		if err != nil {
			return vot, err
		}
		for rows.Next() {
			err = rows.Scan(&vId, &userId, &oldWeight, &submittedAt)
			if err != nil {
				return vot, err
			}
		}
		vot.SubmittedAt = submittedAt
	}

	v := vote{}
	var q string
	if vId != 0 {
		if vot.Weight != 0 && math.Signbit(float64(oldWeight)) == math.Signbit(float64(vot.Weight)) {
			vot.Weight = 0
		}
		q = `update "votes" set "updated_at" = now(), "weight" = $1, "flags" = $2::bit(8) where "item_id" = (select "id" from "content_items" where "key" ~* $3) and "submitted_by" = (select "id" from "accounts" where "key" ~* $4);`
	} else {
		q = `insert into "votes" ("weight", "flags", "item_id", "submitted_by") values ($1, $2::bit(8), (select "id" from "content_items" where "key" ~* $3), (select "id" from "accounts" where "key" ~* $4))`
	}
	v.Flags = vot.Flags
	v.Weight = int(vot.Weight * ScoreMultiplier)

	res, err := db.Exec(q, v.Weight, FlagsNone, vot.Item.Hash, vot.SubmittedBy.Hash)
	if err != nil {
		return vot, err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return vot, errors.Errorf("scoring failed")
	}

	upd := `update "content_items" set score = score - $1 + $2 where "id" = (select "id" from "content_items" where "key" ~* $3)`
	res, err = db.Exec(upd, v.Weight, v.Weight, vot.Item.Hash)
	if err != nil {
		return vot, err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return vot, errors.Errorf("content not found")
	}
	if rows, _ := res.RowsAffected(); rows > 1 {
		return vot, errors.Errorf("content collision")
	}
	log.WithFields(log.Fields{
		"hash":      vot.Item.Hash,
		"oldWeight": oldWeight,
		"newWeight": vot.Weight,
		"voter":     vot.SubmittedBy.Hash,
	}).Infof("vote updated successfully")

	return vot, nil
}

func (i ItemCollection) First() (*Item, error) {
	for _, it := range i {
		return &it, nil
	}
	return nil, errors.Errorf("empty %T", i)
}
