package app

import (
	"database/sql/driver"
	"fmt"
	"github.com/go-ap/errors"
	"github.com/mmcloughlin/meow"
	"html/template"
	"strings"

	mark "gitlab.com/golang-commonmark/markdown"
)

type FlagBits uint8

const (
	FlagsDeleted = FlagBits(1 << iota)

	FlagsNone = FlagBits(0)
)

const MimeTypeURL = MimeType("application/url")
const MimeTypeHTML = MimeType("text/html")
const MimeTypeMarkdown = MimeType("text/markdown")
const MimeTypeText = MimeType("text/plain")
const RandomSeedSelectedByDiceRoll = 777

type Key [32]byte

func (k Key) IsEmpty() bool {
	return k == Key{}
}

func (k Key) String() string {
	return string(k[0:32])
}
func (k Key) Bytes() []byte {
	return []byte(k[0:32])
}
func (k Key) Hash() Hash {
	return Hash(k[0:32])
}

func (k *Key) FromBytes(s []byte) error {
	var err error
	if len(s) > 32 {
		err = errors.Errorf("incoming byte array %q longer than expected ", s)
	}
	if len(s) < 32 {
		err = errors.Errorf("incoming byte array %q longer than expected ", s)
	}
	for i := range s {
		k[i] = s[i]
	}
	return err
}
func (k *Key) FromString(s string) error {
	var err error
	if len(s) > 32 {
		err = errors.Errorf("incoming string %q longer than expected ", s)
	}
	if len(s) < 32 {
		err = errors.Errorf("incoming string %q longer than expected ", s)
	}
	for i := range s {
		k[i] = s[i]
	}
	return err
}

// Value implements the driver.Valuer interface
func (k Key) Value() (driver.Value, error) {
	if len(k) > 0 {
		return k.String(), nil
	}
	return nil, nil
}

// Scan implements the sql.Scanner interface
func (k *Key) Scan(src interface{}) error {
	if v, ok := src.([]byte); ok {
		k.FromBytes(v)
	} else {
		return errors.Errorf("bad []byte type assertion when loading %T", k)
	}

	return nil
}
func (f *FlagBits) FromInt64() error {
	return nil
}

type ItemCollection []Item

func GenKey(el ...[]byte) Key {
	lim := []byte("##")

	buf := strings.Builder{}
	for i, l := range el {
		buf.Write(l)
		if i < len(el)-1 {
			buf.Write(lim)
		}
	}

	var k Key
	k.FromString(fmt.Sprintf("%x", meow.Checksum(RandomSeedSelectedByDiceRoll, []byte(buf.String()))))
	return k
}

func Markdown(data string) template.HTML {
	md := mark.New(
		mark.HTML(true),
		mark.Tables(true),
		mark.Linkify(false),
		mark.Breaks(false),
		mark.Typographer(true),
		mark.XHTMLOutput(false),
	)

	h := md.RenderToString([]byte(data))
	return template.HTML(h)
}

// HasMetadata
func (i Item) HasMetadata() bool {
	return i.Metadata != nil
}

// IsFederated
func (i Item) IsFederated() bool {
	return !i.IsLocal()
}

// IsLocal
func (i Item) IsLocal() bool {
	if !i.HasMetadata() {
		return true
	}
	if len(i.Metadata.ID) > 0 {
		return HostIsLocal(i.Metadata.ID)
	}
	if len(i.Metadata.URL) > 0 {
		return HostIsLocal(i.Metadata.URL)
	}
	return true
}
