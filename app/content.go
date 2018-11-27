package app

import (
	"database/sql/driver"
	"fmt"
	"github.com/juju/errors"
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

const MimeTypeURL = "application/url"
const MimeTypeHTML = "text/html"
const MimeTypeMarkdown = "text/markdown"
const MimeTypeText = "text/plain"

type Key [32]byte

func (k Key) String() string {
	return string(k[0:32])
}
func (k Key) Bytes() []byte {
	return []byte(k[0:32])
}
func (k Key) Hash() Hash {
	return Hash(k[0:10])
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

// Value implements the driver.Valuer interface,
// and turns the Key into a bitfield (BIT(8)) storage.
func (k Key) Value() (driver.Value, error) {
	if len(k) > 0 {
		return k.Bytes(), nil
	}
	return []byte{0}, nil
}

// Scan implements the sql.Scanner interface,
// and turns the bitfield incoming from DB into a Key
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

func GenKey(elems ...[]byte) Key {
	lim := []byte("##")

	buf := strings.Builder{}
	for i, el := range elems {
		buf.Write(el)
		if i < len(elems)-1 {
			buf.Write(lim)
		}
	}

	var k Key
	k.FromString(fmt.Sprintf("%x", meow.Checksum(777, []byte(buf.String()))))
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
