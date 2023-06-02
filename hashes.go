package brutalinks

import (
	"path"
	"strings"

	vocab "github.com/go-ap/activitypub"
	"github.com/google/uuid"
)

// Hash is a local type for string, it should hold a [32]byte array actually
type Hash uuid.UUID

// AnonymousHash is the sha hash for the anonymous account
var (
	AnonymousHash = Hash{}
	SystemHash    = Hash{0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
)

func HashFromIRI(i vocab.IRI) Hash {
	_, h := path.Split(strings.TrimRight(i.String(), "/"))
	return HashFromString(h)
}

func HashFromItem(obj vocab.Item) Hash {
	if obj == nil {
		return AnonymousHash
	}
	iri := obj.GetLink()
	if len(iri) == 0 {
		return AnonymousHash
	}
	return HashFromIRI(iri)
}

func HashFromString(s string) Hash {
	if u, err := uuid.Parse(s); err == nil {
		return Hash(u)
	}
	return AnonymousHash
}

// String returns the hash as a string
func (h Hash) String() string {
	return uuid.UUID(h).String()
}

// MarshalText
func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

func (h Hash) IsValid() bool {
	return uuid.UUID(h).Time() > 0
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
