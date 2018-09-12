package db

import (
	"database/sql/driver"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/mariusor/littr.go/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	"github.com/juju/errors"
)

type config struct {
	DB *sqlx.DB
}

// I think we can move from using the exported Config package variable
// to an unexported one. First we need to decouple the DB config from the repository struct to a config struct
var Config config

type (
	Key      [64]byte
	FlagBits [8]byte
	Metadata types.JSONText
)

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

func (m Metadata) MarshalJSON() ([]byte, error) {
	return types.JSONText(m).MarshalJSON()
}

func (m *Metadata) UnmarshalJSON(data []byte) error {
	j := &types.JSONText{}
	err := j.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	*m = Metadata(*j)
	return nil
}

func AccountFlags(f FlagBits) models.FlagBits {
	return VoteFlags(f)
}

func ItemMetadata(m Metadata) models.ItemMetadata {
	return models.ItemMetadata(m)
}

func AccountMetadata(m Metadata) models.AccountMetadata {
	am := models.AccountMetadata{}
	json.Unmarshal([]byte(m), &am)
	return am
}

func (a Account) Model() models.Account {
	m := AccountMetadata(a.Metadata)
	f := AccountFlags(a.Flags)
	return models.Account{
		Email:     string(a.Email),
		Handle:    a.Handle,
		Hash:      a.Key.String(),
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
		Score:     a.Score,
		Flags:     f,
		Metadata:  &m,
	}
}

// Value implements the driver.Valuer interface,
// and turns the FlagBits into a bitfield (BIT(8)) storage.
func (f FlagBits) Value() (driver.Value, error) {
	if len(f) > 0 {
		return []byte(f[0:8]), nil
	}
	return []byte{0}, nil
}

// Scan implements the sql.Scanner interface,
// and turns the bitfield incoming from DB into a FlagBits
func (f *FlagBits) Scan(src interface{}) error {
	if v, ok := src.([]byte); ok {
		for j, bit := range v {
			f[j] = uint8(bit - 0x40)
		}
	} else {
		return errors.Errorf("bad %T type assertion when loading %T", v, f)
	}
	return nil
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

func trimHash(s string) string {
	h, err := url.PathUnescape(s)
	if err != nil {
		return ""
	}
	h = strings.TrimSpace(h)
	if len(h) == 0 {
		return ""
	}
	return h
}

func (c config) LoadVotes(f models.LoadVotesFilter) (models.VoteCollection, error) {
	return LoadVotes(c.DB, f)
}

func (c config) LoadVote(f models.LoadVotesFilter) (models.Vote, error) {
	f.MaxItems = 1
	votes, err := LoadVotes(c.DB, f)
	if err != nil {
		return models.Vote{}, err
	}
	v, err := votes.First()
	return *v, err
}
