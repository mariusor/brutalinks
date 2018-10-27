package models

import (
	"github.com/juju/errors"
)

type FlagBits uint8

const (
	FlagsDeleted = FlagBits(1 << iota)

	FlagsNone = FlagBits(0)
)
const MimeTypeURL = "application/url"

type Key [32]byte

func (k Key) String() string {
	return string(k[0:32])
}
func (k Key) Bytes() []byte {
	return []byte(k[0:32])
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

func (f *FlagBits) FromInt64() error {
	return nil
}

type ItemCollection []Item
