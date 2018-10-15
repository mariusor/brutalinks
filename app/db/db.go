package db

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	"github.com/juju/errors"
)

type config struct {
	DB *sqlx.DB
}

func init() {
	if Logger == nil {
		Logger = log.StandardLogger()
	}
}

// I think we can move from using the exported Config package variable
// to an unexported one. First we need to decouple the DB config from the repository struct to a config struct
var Config config

// Repository middleware
func Repository(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		newCtx := context.WithValue(ctx, models.RepositoryCtxtKey, Config)
		next.ServeHTTP(w, r.WithContext(newCtx))
	}
	return http.HandlerFunc(fn)
}

type (
	Key      [64]byte
	FlagBits [8]byte
	Metadata types.JSONText
)

func (k Key) Hash() models.Hash {
	return models.Hash(k[0:10])
}
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
		Hash:      a.Key.Hash(),
		Email:     string(a.Email),
		Handle:    a.Handle,
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
	return loadVotes(c.DB, f)
}

func (c config) LoadVote(f models.LoadVotesFilter) (models.Vote, error) {
	f.MaxItems = 1
	votes, err := loadVotes(c.DB, f)
	if err != nil {
		return models.Vote{}, err
	}
	if v, err := votes.First(); err == nil {
		return *v, nil
	} else {
		return models.Vote{}, err
	}
}

func (c config) SaveVote(v models.Vote) (models.Vote, error) {
	return saveVote(c.DB, v)
}

func (c config) SaveItem(it models.Item) (models.Item, error) {
	return saveItem(c.DB, it)
}

func (c config) LoadItem(f models.LoadItemsFilter) (models.Item, error) {
	f.MaxItems = 1
	items, err := loadItems(c.DB, f)
	if err != nil {
		return models.Item{}, err
	}
	if i, err := items.First(); err == nil {
		return *i, nil
	} else {
		return models.Item{}, err
	}
}

func (c config) LoadItems(f models.LoadItemsFilter) (models.ItemCollection, error) {
	return loadItems(c.DB, f)
}

func (c config) LoadAccount(f models.LoadAccountsFilter) (models.Account, error) {
	f.MaxItems = 1
	accounts, err := loadAccounts(c.DB, f)
	if err != nil {
		return models.Account{}, err
	}
	if a, err := accounts.First(); err == nil {
		return *a, nil
	} else {
		return models.Account{}, err
	}
}

func (c config) LoadAccounts(f models.LoadAccountsFilter) (models.AccountCollection, error) {
	return loadAccounts(c.DB, f)
}

func (c config) SaveAccount(a models.Account) (models.Account, error) {
	return saveAccount(c.DB, a)
}

func LoadScoresForItems(since time.Duration, key string) ([]models.Score, error) {
	return loadScoresForItems(Config.DB, since, key)
}

func loadScoresForItems(db *sqlx.DB, since time.Duration, key string) ([]models.Score, error) {
	par := make([]interface{}, 0)
	par = append(par, interface{}(since.Hours()))

	keyClause := ""
	if len(key) > 0 {
		keyClause = ` and "content_items"."key" ~* $2`
		par = append(par, interface{}(key))
	}
	q := fmt.Sprintf(`select "item_id", "content_items"."key", max("content_items"."submitted_at"),
		sum(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
		sum(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
		from "votes" inner join "content_items" on "content_items"."id" = "item_id"
		where current_timestamp - "content_items"."submitted_at" < ($1 * INTERVAL '1 hour')%s group by "item_id", "key" order by "item_id";`,
		keyClause)
	rows, err := db.Query(q, par...)
	if err != nil {
		return nil, err
	}
	scores := make([]models.Score, 0)
	for rows.Next() {
		var i, ups, downs int64
		var submitted time.Time
		var key []byte
		err = rows.Scan(&i, &key, &submitted, &ups, &downs)
		dumb := func(ups, downs int64) int64 {
			return ups - downs
		}
		now := time.Now()
		reddit := int64(models.Reddit(ups, downs, now.Sub(submitted)))
		wilson := int64(models.Wilson(ups, downs))
		hacker := int64(models.Hacker(ups-downs, now.Sub(submitted)))
		dumbScore := dumb(ups, downs)
		Logger.WithFields(log.Fields{}).Infof("Votes[%s]: UPS[%d] DOWNS[%d] - new score R%d:W%d:H%d:D%d", key[0:8], ups, downs, reddit, wilson, hacker, dumbScore)
		new := models.Score{
			ID:        i,
			Key:       key,
			Submitted: submitted,
			Type:      models.ScoreItem,
			Score:     dumbScore,
		}
		scores = append(scores, new)
	}
	return scores, nil
}

func LoadScoresForAccounts(since time.Duration, col string, val string) ([]models.Score, error) {
	return loadScoresForAccounts(Config.DB, since, col, val)
}

func loadScoresForAccounts(db *sqlx.DB, since time.Duration, col string, val string) ([]models.Score, error) {
	par := make([]interface{}, 0)
	par = append(par, interface{}(since.Hours()))

	keyClause := ""
	if len(val) > 0 && len(col) > 0 {
		keyClause = fmt.Sprintf(` and "content_items"."%s" ~* $2`, col)
		par = append(par, interface{}(val))
	}
	q := fmt.Sprintf(`select "accounts"."id", "accounts"."handle", "accounts"."key", max("content_items"."submitted_at"),
       sum(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
       sum(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
from "votes"
       inner join "content_items" on "content_items"."id" = "item_id"
       inner join "accounts" on "content_items"."submitted_by" = "accounts"."id"
where current_timestamp - "content_items"."submitted_at" < ($1 * INTERVAL '1 hour')%s
group by "accounts"."id", "accounts"."key" order by "accounts"."id";`,
		keyClause)
	rows, err := db.Query(q, par...)
	if err != nil {
		return nil, err
	}

	scores := make([]models.Score, 0)
	for rows.Next() {
		var i, ups, downs int64
		var submitted time.Time
		var key []byte
		var handle string
		err = rows.Scan(&i, &handle, &key, &submitted, &ups, &downs)

		dumb := func(ups, downs int64) int64 {
			return ups - downs
		}
		now := time.Now()
		reddit := int64(models.Reddit(ups, downs, now.Sub(submitted)))
		wilson := int64(models.Wilson(ups, downs))
		hacker := int64(models.Hacker(ups-downs, now.Sub(submitted)))
		dumbScore := dumb(ups, downs)
		Logger.WithFields(log.Fields{}).Infof("Votes[%s]: UPS[%d] DOWNS[%d] - new score R%d:W%d:H%d:D%d", handle, ups, downs, reddit, wilson, hacker, dumbScore)
		new := models.Score{
			ID:        i,
			Key:       key,
			Submitted: submitted,
			Type:      models.ScoreAccount,
			Score:     dumbScore,
		}
		scores = append(scores, new)
	}
	return scores, nil
}
