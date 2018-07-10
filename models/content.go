package models

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
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
		if len(c.Path) == 0 {
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
	if len(c.Path) > 0 {
		parentHash := c.Path[0:8]
		return fmt.Sprintf("/op/%s/%s", c.Hash(), parentHash)
	}
	return "/"
}

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
