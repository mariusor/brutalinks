package models

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
)

type FlagBits uint8

const (
	FlagsDeleted = FlagBits(1 << iota)

	FlagsNone = FlagBits(0)
)
const MimeTypeURL = "application/url"

type Key [64]byte

func (k Key) String() string {
	return string(k[0:64])
}
func (k Key) Bytes() []byte {
	return []byte(k[0:64])
}

func (k *Key) FromBytes(s []byte) error {
	var err error
	if len(s) > 64 {
		err = errors.Errorf("incoming byte array %q longer than expected ", s)
	}
	if len(s) < 64 {
		err = errors.Errorf("incoming byte array %q longer than expected ", s)
	}
	for i := range s {
		k[i] = s[i]
	}
	return err
}
func (k *Key) FromString(s string) error {
	var err error
	if len(s) > 64 {
		err = errors.Errorf("incoming string %q longer than expected ", s)
	}
	if len(s) < 64 {
		err = errors.Errorf("incoming string %q longer than expected ", s)
	}
	for i := range s {
		k[i] = s[i]
	}
	return err
}

func (f *FlagBits) FromInt64() error {
	return nil
}

type item struct {
	ID                 int64     `orm:id,"auto"`
	Key                Key       `orm:key,size(64)`
	Title              []byte    `orm:title`
	MimeType           string    `orm:mime_type`
	Data               []byte    `orm:data`
	Score              int64     `orm:score`
	SubmittedAt        time.Time `orm:created_at`
	SubmittedBy        int64     `orm:submitted_by`
	UpdatedAt          time.Time `orm:updated_at`
	Flags              FlagBits  `orm:flags`
	Metadata           []byte    `orm:metadata`
	Path               []byte    `orm:path`
	SubmittedByAccount *Account
	fullPath           []byte
	parentLink         string
}

type ItemCollection []Item

func getAncestorHash(path []byte, cnt int) []byte {
	if path == nil {
		return nil
	}
	elem := bytes.Split(path, []byte("."))
	l := len(elem)
	if cnt > l || cnt < 0 {
		cnt = l
	}
	return elem[l-cnt]
}

func (c item) GetParentHash() Hash {
	if c.IsTop() {
		return ""
	}
	return Hash(getAncestorHash(c.Path, 1))
}
func (c item) GetOPHash() Hash {
	if c.IsTop() {
		return ""
	}
	return Hash(getAncestorHash(c.Path, -1))
}

func (c item) IsSelf() bool {
	mimeComponents := strings.Split(c.MimeType, "/")
	return mimeComponents[0] == "text"
}

func (c *item) GetKey() Key {
	data := c.Data
	now := c.UpdatedAt
	if now.IsZero() {
		now = time.Now()
	}
	data = append(data, []byte(fmt.Sprintf("%d", now.UnixNano()))...)
	data = append(data, []byte(c.Path)...)
	data = append(data, []byte(fmt.Sprintf("%d", c.SubmittedBy))...)

	c.Key.FromString(fmt.Sprintf("%x", sha256.Sum256(data)))
	return c.Key
}
func (c item) IsTop() bool {
	return c.Path == nil || len(c.Path) == 0
}
func (c item) Hash() Hash {
	return c.Hash8()
}
func (c item) Hash8() Hash {
	if len(c.Key) > 8 {
		return Hash(c.Key[0:8])
	}
	return Hash(c.Key.String())
}
func (c item) Hash16() Hash {
	if len(c.Key) > 16 {
		return Hash(c.Key[0:16])
	}
	return Hash(c.Key.String())
}
func (c item) Hash32() Hash {
	if len(c.Key) > 32 {
		return Hash(c.Key[0:32])
	}
	return Hash(c.Key.String())
}
func (c item) Hash64() Hash {
	return Hash(c.Key.String())
}

func (c *item) FullPath() []byte {
	if len(c.fullPath) == 0 {
		c.fullPath = append(c.fullPath, c.Path...)
		if len(c.fullPath) > 0 {
			c.fullPath = append(c.fullPath, byte('.'))
		}
		c.fullPath = append(c.fullPath, c.Key.Bytes()...)
	}
	return c.fullPath
}

func (c item) Deleted() bool {
	return c.Flags&FlagsDeleted == FlagsDeleted
}
func (c item) UnDelete() {
	c.Flags ^= FlagsDeleted
}
func (c *item) Delete() {
	c.Flags &= FlagsDeleted
}
func (c item) IsLink() bool {
	return c.MimeType == MimeTypeURL
}
func (c item) GetDomain() string {
	if !c.IsLink() {
		return ""
	}
	return strings.Split(string(c.Data), "/")[2]
}
