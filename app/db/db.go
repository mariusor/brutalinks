package db

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/log"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	"github.com/juju/errors"
)

type config struct {
	Account *app.Account
	DB      *sqlx.DB
}

func Init(app *app.Application) error {
	if app.Config.DB.Port == "" {
		app.Config.DB.Port = "5432"
	}
	connStr := fmt.Sprintf("host=%s user=%s password=%s port=%s dbname=%s sslmode=disable",
		app.Config.DB.Host, app.Config.DB.User, app.Config.DB.Pw, app.Config.DB.Port, app.Config.DB.Name)

	var err error
	Config.DB, err = sqlx.Open("postgres", connStr)
	if err == nil {
		app.Config.DB.Enabled = true
	} else {
		new := errors.NewErr("failed to connect to the database")
		app.Logger.WithContext(log.Ctx{
			"dbHost":   app.Config.DB.Host,
			"dbPort":   app.Config.DB.Port,
			"dbName":   app.Config.DB.Name,
			"dbUser":   app.Config.DB.User,
			"previous": err,
			"trace":    new.StackTrace(),
		}).Error(new.Error())
	}
	return err
}

// I think we can move from using the exported Config package variable
// to an unexported one. First we need to decouple the DB config from the repository struct to a config struct
var Config config

// Repository middleware
func Repository(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		newCtx := context.WithValue(ctx, app.RepositoryCtxtKey, Config)
		next.ServeHTTP(w, r.WithContext(newCtx))
	}
	return http.HandlerFunc(fn)
}

type (
	FlagBits [8]byte
	Metadata types.JSONText
)

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

func AccountFlags(f FlagBits) app.FlagBits {
	return VoteFlags(f)
}

func ItemMetadata(m Metadata) (app.ItemMetadata, error) {
	am := app.ItemMetadata{}
	err := json.Unmarshal(m, &am)
	return am, err
}

func AccountMetadata(m Metadata) (app.AccountMetadata, error) {
	am := app.AccountMetadata{}
	err := json.Unmarshal(m, &am)
	return am, err
}

func (a Account) Model() app.Account {
	m, _ := AccountMetadata(a.Metadata)
	f := AccountFlags(a.Flags)
	return app.Account{
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
	l := len(f)
	if l > 0 {
		var val string
		for _, b := range f {
			if b == 1 {
				val += "1"
			} else {
				val += "0"
			}
		}
		return val, nil
	}
	return 0, nil
}

// Scan implements the sql.Scanner interface,
// and turns the bitfield incoming from DB into a FlagBits
func (f *FlagBits) Scan(src interface{}) error {
	switch v := src.(type) {
	case []byte:
		for j, bit := range v {
			f[j] = uint8(bit - 0x30)
		}
	case app.FlagBits:
		for j := range f {
			bv := v >> uint(len(f)-j-1)
			f[j] = uint8(bv)
		}
	default:
		return errors.Errorf("bad %T type assertion when loading %T", v, f)
	}
	return nil
}

func (c config) WithAccount(a *app.Account) error {
	c.Account = a
	// @todo(marius): implement this
	return errors.NotImplementedf("db.Config.WithAccount")
}

func (c config) LoadVotes(f app.LoadVotesFilter) (app.VoteCollection, error) {
	return loadVotes(c.DB, f)
}

func (c config) LoadVote(f app.LoadVotesFilter) (app.Vote, error) {
	f.MaxItems = 1
	votes, err := loadVotes(c.DB, f)
	if err != nil {
		return app.Vote{}, err
	}
	if v, err := votes.First(); err == nil {
		return *v, nil
	} else {
		return app.Vote{}, err
	}
}

func (c config) SaveVote(v app.Vote) (app.Vote, error) {
	return saveVote(c.DB, v)
}

func (c config) SaveItem(it app.Item) (app.Item, error) {
	return saveItem(c.DB, it)
}

func (c config) LoadItem(f app.LoadItemsFilter) (app.Item, error) {
	f.MaxItems = 1
	items, err := loadItems(c.DB, f)
	if err != nil {
		return app.Item{}, err
	}
	if i, err := items.First(); err == nil {
		return *i, nil
	} else {
		return app.Item{}, errors.NotFoundf("item %s", f.Key)
	}
}

func (c config) LoadItems(f app.LoadItemsFilter) (app.ItemCollection, error) {
	return loadItems(c.DB, f)
}

func (c config) LoadAccount(f app.LoadAccountsFilter) (app.Account, error) {
	f.MaxItems = 1
	accounts, err := loadAccounts(c.DB, f)
	if err != nil {
		return app.Account{}, err
	}
	if a, err := accounts.First(); err == nil {
		return *a, nil
	} else {
		return app.Account{}, errors.NotFoundf("account %s", f.Key)
	}
}

func (c config) LoadAccounts(f app.LoadAccountsFilter) (app.AccountCollection, error) {
	return loadAccounts(c.DB, f)
}

func (c config) SaveAccount(a app.Account) (app.Account, error) {
	return saveAccount(c.DB, a)
}

func LoadScoresForItems(since time.Duration, key string) ([]app.Score, error) {
	return loadScoresForItems(Config.DB, since, key)
}

var maxVotePeriod, _ = time.ParseDuration("44444h")

func loadScoresForItems(db *sqlx.DB, since time.Duration, key string) ([]app.Score, error) {
	par := make([]interface{}, 0)
	dumb := func(ups, downs int64) int64 {
		return ups - downs
	}
	keyClause := ""
	if len(key) > 0 {
		keyClause = `AND "items"."key" ~* $1`
		par = append(par, interface{}(key))
	}
	q := fmt.Sprintf(`select "item_id", "items"."key", max("items"."submitted_at"),
		sum(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
		sum(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
		FROM "votes" INNER JOIN "items" ON "items"."id" = "item_id"
		WHERE "votes"."updated_at" >= current_timestamp - INTERVAL '%.3f hours' %s 
	GROUP BY "item_id", "key" ORDER BY "item_id";`,
		since.Hours(), keyClause)
	rows, err := db.Query(q, par...)
	if err != nil {
		return nil, err
	}
	scores := make([]app.Score, 0)
	for rows.Next() {
		var i, ups, downs int64
		var submitted time.Time
		var key []byte
		err = rows.Scan(&i, &key, &submitted, &ups, &downs)

		now := time.Now().UTC()
		reddit := int64(app.Reddit(ups, downs, now.Sub(submitted)))
		wilson := int64(app.Wilson(ups, downs))
		hacker := int64(app.Hacker(ups-downs, now.Sub(submitted)))
		dumbScore := dumb(ups, downs)
		Logger.WithContext(log.Ctx{
			"key":    string(key[0:8]),
			"ups":    ups,
			"downs":  downs,
			"reddit": reddit,
			"wilson": wilson,
			"hn":     hacker,
			"dumb":   dumbScore,
		}).Info("new score")
		new := app.Score{
			ID:        i,
			Key:       key,
			Submitted: submitted,
			Type:      app.ScoreItem,
			Score:     dumbScore,
		}
		scores = append(scores, new)
	}
	return scores, nil
}

func LoadScoresForAccounts(since time.Duration, col string, val string) ([]app.Score, error) {
	return loadScoresForAccounts(Config.DB, since, col, val)
}

func loadScoresForAccounts(db *sqlx.DB, since time.Duration, col string, val string) ([]app.Score, error) {
	par := make([]interface{}, 0)
	dumb := func(ups, downs int64) int64 {
		return ups - downs
	}
	keyClause := ""
	if len(val) > 0 && len(col) > 0 {
		keyClause = fmt.Sprintf(` and "items"."%s" ~* $1`, col)
		par = append(par, interface{}(val))
	}
	q := fmt.Sprintf(`SELECT "accounts"."id", "accounts"."handle", "accounts"."key", max("items"."submitted_at"),
       SUM(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
       SUM(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
FROM "votes"
       INNER JOIN "items" ON "items"."id" = "item_id"
       INNER JOIN "accounts" ON "items"."submitted_by" = "accounts"."id"
WHERE "votes"."updated_at" >= current_timestamp - INTERVAL '%.3f hours' %s 
GROUP BY "accounts"."id", "accounts"."key" ORDER BY "accounts"."id";`,
		since.Hours(), keyClause)
	rows, err := db.Query(q, par...)
	if err != nil {
		return nil, err
	}

	scores := make([]app.Score, 0)
	for rows.Next() {
		var i, ups, downs int64
		var submitted time.Time
		var key []byte
		var handle string
		err = rows.Scan(&i, &handle, &key, &submitted, &ups, &downs)

		now := time.Now().UTC()
		reddit := int64(app.Reddit(ups, downs, now.Sub(submitted)))
		wilson := int64(app.Wilson(ups, downs))
		hacker := int64(app.Hacker(ups-downs, now.Sub(submitted)))
		dumbScore := dumb(ups, downs)
		Logger.WithContext(log.Ctx{
			"handle": handle,
			"ups":    ups,
			"downs":  downs,
			"reddit": reddit,
			"wilson": wilson,
			"hn":     hacker,
			"dumb":   dumbScore,
		}).Info("new score")
		new := app.Score{
			ID:        i,
			Key:       key,
			Submitted: submitted,
			Type:      app.ScoreAccount,
			Score:     dumbScore,
		}
		scores = append(scores, new)
	}
	return scores, nil
}

// LoadInfo this method is here to keep compatibility with the repository interfaces
// but in the long term we might want to store some of this information in the db
func (c config) LoadInfo() (app.Info, error) {
	return app.Instance.NodeInfo(), nil
}
