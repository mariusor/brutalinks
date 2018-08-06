package models

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
	"database/sql"
	)

const (
	FlagsNone    = 0
	FlagsDeleted = 1
	MimeTypeURL  = "application/url"
)

type Content struct {
	Id          int64     `orm:Id,"auto"`
	Key         []byte    `orm:key,size(64)`
	Title       []byte    `orm:title`
	MimeType    string    `orm:mime_type`
	Data        []byte    `orm:data`
	Score       int64     `orm:score`
	SubmittedAt time.Time `orm:created_at`
	SubmittedBy int64     `orm:submitted_by`
	UpdatedAt   time.Time `orm:updated_at`
	Flags       int8      `orm:Flags`
	Metadata    []byte    `orm:metadata`
	Path        []byte    `orm:path`
	SubmittedByAccount *Account
	fullPath    []byte
	parentLink  string
}

type Item interface {
	Id() int64
}

type ContentCollection []Content

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
func (c Content) IsTop() bool {
	return c.Path == nil || len(c.Path) == 0
}
func (c Content) Hash() string {
	return c.Hash8()
}
func (c Content) Hash8() string {
	if len(c.Key) > 8 {
		return string(c.Key[0:8])
	}
	return string(c.Key)
}
func (c Content) Hash16() string {
	if len(c.Key) > 16 {
		return string(c.Key[0:16])
	}
	return string(c.Key)
}
func (c Content) Hash32() string {
	if len(c.Key) > 32 {
 	return string(c.Key[0:32])
	}
	return string(c.Key)
}
func (c Content) Hash64() string {
	return string(c.Key)
}

func (c *Content) FullPath() []byte {
	if len(c.fullPath) == 0 {
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
func (c Content) GetDomain() string {
	if !c.IsLink() {
		return ""
	}
	return strings.Split(string(c.Data), "/")[2]
}

func LoadItem(db *sql.DB, h string) (Content, error) {
	p := Content{}
	sel := `select 
			"content_items"."id", "content_items"."key", "content_items"."mime_type", "content_items"."data", 
			"content_items"."title", "content_items"."score", "content_items"."submitted_at", 
			"content_items"."submitted_by", "content_items"."flags", "content_items"."metadata", "content_items"."path",
			"accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score", 
			"accounts"."created_at", "accounts"."metadata", "accounts"."flags"
		from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
		where "content_items"."key" ~* $1`
	rows, err := db.Query(sel, h)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		a := Account{}
		err := rows.Scan(
			&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
			&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return p, err
		}
		p.SubmittedByAccount = &a
	}
	return p, nil
}
func LoadOPItems(db *sql.DB, max int) ([]Content, error) {
	items := make([]Content, 0)
	sel := fmt.Sprintf(`select 
			"content_items"."id", "content_items"."key", "content_items"."mime_type", "content_items"."data", 
			"content_items"."title", "content_items"."score", "content_items"."submitted_at", 
			"content_items"."submitted_by", "content_items"."flags", "content_items"."metadata", "content_items"."path",
			"accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score", 
			"accounts"."created_at", "accounts"."metadata", "accounts"."flags"
		from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
		where "path" is NULL or nlevel("path") = 0
	order by "content_items"."score" desc, "content_items"."submitted_at" desc limit %d`, max)
	rows, err := db.Query(sel)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		p := Content{}
		a := Account{}
		err := rows.Scan(
			&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
			&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return nil, err
		}
		p.SubmittedByAccount = &a
		items = append(items, p)
	}

	return items, nil
}

func LoadItemsByPath(db *sql.DB, path []byte, max int) ([]Content, error) {
	items := make([]Content, 0)
	sel := fmt.Sprintf(`select 
			"content_items"."id", "content_items"."key", "content_items"."mime_type", "content_items"."data", 
			"content_items"."title", "content_items"."score", "content_items"."submitted_at", 
			"content_items"."submitted_by", "content_items"."flags", "content_items"."metadata", "content_items"."path",
			"accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score", 
			"accounts"."created_at", "accounts"."metadata", "accounts"."flags"
		from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
		where "content_items"."path" <@ $1 and "content_items"."path" is not null order by "content_items"."path" asc, 
			"content_items"."score" desc, "content_items"."submitted_at" desc limit %d`, max)
	rows, err := db.Query(sel, path)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		p := Content{}
		a := Account{}
		err := rows.Scan(
			&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
			&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return nil, err
		}
		p.SubmittedByAccount = &a
		items = append(items, p)
	}

	return items, nil
}

func LoadItemsByDomain(db *sql.DB, domain string, max int) ([]Content, error) {
	items := make([]Content, 0)
	sel := fmt.Sprintf(`select 
			"content_items"."id", "content_items"."key", "content_items"."mime_type", "content_items"."data", 
			"content_items"."title", "content_items"."score", "content_items"."submitted_at", 
			"content_items"."submitted_by", "content_items"."flags", "content_items"."metadata", "content_items"."path",
			"accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score", 
			"accounts"."created_at", "accounts"."metadata", "accounts"."flags"
		from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
		where "content_items"."mime_type" = $1 
			AND substring("content_items"."data"::text from 'http[s]?://([^/]*)') = $2 order by "content_items"."submitted_at" desc limit %d`, max)
	rows, err := db.Query(sel, MimeTypeURL, domain)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		p := Content{}
		a := Account{}
		err := rows.Scan(
			&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
			&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return nil, err
		}
		p.SubmittedByAccount = &a
		items = append(items, p)
	}

	return items, nil
}
func LoadItemByHash(db *sql.DB, hash string) (Content, error) {
	p := Content{}
	a := Account{}

	sel := `select "content_items"."id", "content_items"."key", "content_items"."mime_type", "content_items"."data", 
			"content_items"."title", "content_items"."score", "content_items"."submitted_at", 
			"content_items"."submitted_by", "content_items"."flags", "content_items"."metadata", "content_items"."path",
			"accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score", 
			"accounts"."created_at", "accounts"."metadata", "accounts"."flags"
 			from "content_items"
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by"
			where "content_items"."key" ~* $1`
	rows, err := db.Query(sel, hash)
	if err != nil {
		return p, err
	}

	for rows.Next() {
		err = rows.Scan(&p.Id, &p.Key, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
			&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return p, err
		}
		p.SubmittedByAccount = &a
	}
	return p, nil
}
