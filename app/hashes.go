package app

import (
	"github.com/google/uuid"
	"strings"
)

// Hash is a local type for string, it should hold a [32]byte array actually
type Hash uuid.UUID

// AnonymousHash is the sha hash for the anonymous account
var AnonymousHash = Hash{}

func HashFromString(s string) Hash {
	if u, err := uuid.Parse(s); err == nil {
		return Hash(u)
	}
	return Hash{}
}

// String returns the hash as a string
func (h Hash) String() string {
	return uuid.UUID(h).String()
}

// MarshalText
func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

func (h Hash)Valid() bool {
	return uuid.UUID(h).ID() > 0
}

type Hashes []Hash

func (h Hashes) Contains(s Hash) bool {
	for _, hh := range h {
		if hh == s {
			return true
		}
	}
	return false
}

func (h Hashes) String() string {
	str := make([]string, len(h))
	for i, hh := range h {
		str[i] = hh.String()
	}
	return strings.Join(str, ", ")
}

