package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/internal/log"
	"net/http"
	"time"

	"github.com/go-pg/pg"
	"github.com/mariusor/littr.go/internal/errors"
)

type config struct {
	Account *app.Account
	DB      *pg.DB
}

func Init(app *app.Application) error {
	if app.Config.DB.Port == "" {
		app.Config.DB.Port = "5432"
	}

	var err error
	Config.DB = pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", app.Config.DB.Host, app.Config.DB.Port),
		User:     app.Config.DB.User,
		Password: app.Config.DB.Pw,
		Database: app.Config.DB.Name,
	})
	if err == nil {
		app.Config.DB.Enabled = true
	} else {
		new := errors.New("failed to connect to the database")
		app.Logger.WithContext(log.Ctx{
			"dbHost":   app.Config.DB.Host,
			"dbPort":   app.Config.DB.Port,
			"dbName":   app.Config.DB.Name,
			"dbUser":   app.Config.DB.User,
			"previous": err,
			//"trace":    new.StackTrace(),
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
	Path     []byte
)

func AccountFlags(f FlagBits) app.FlagBits {
	return VoteFlags(f)
}

func (a Account) Model() app.Account {
	m := a.Metadata
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

// Value implements the driver.Valuer interface
func (p Path) Value() (driver.Value, error) {
	return driver.Value(p), nil
}

// Scan implements the sql.Scanner interface,
func (p *Path) Scan(src interface{}) error {
	s := sql.NullString{}
	err := s.Scan(src)
	if s.Valid {
		*p = Path(s.String)
	} else {
		*p = nil
	}
	return err
}
func (c config) WithAccount(a *app.Account) error {
	c.Account = a
	// @todo(marius): implement this
	return errors.NotImplementedf("DB.Config.WithAccount")
}

func (c config) LoadVotes(f app.LoadVotesFilter) (app.VoteCollection, uint, error) {
	var count uint = 0
	var err error
	var votes app.VoteCollection

	if votes, err = loadVotes(c.DB, f); err == nil {
		count, err = countVotes(c.DB, f)
		return votes, count, err
	}
	return votes, count, err
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

func (c config) LoadItems(f app.LoadItemsFilter) (app.ItemCollection, uint, error) {
	var count uint = 0
	var err error
	var items app.ItemCollection

	if items, err = loadItems(c.DB, f); err == nil {
		count, err = countItems(c.DB, f)
		return items, count, err
	}
	return items, count, err
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

func (c config) LoadAccounts(f app.LoadAccountsFilter) (app.AccountCollection, uint, error) {
	var count uint = 0
	var err error
	var accounts app.AccountCollection

	if accounts, err = loadAccounts(c.DB, f); err == nil {
		count, err = countAccounts(c.DB, f)
		return accounts, count, err
	}
	return accounts, count, err
}

func (c config) SaveAccount(a app.Account) (app.Account, error) {
	return saveAccount(c.DB, a)
}

func LoadScoresForItems(since time.Duration, key string) ([]app.Score, error) {
	return loadScoresForItems(Config.DB, since, key)
}

var maxVotePeriod, _ = time.ParseDuration("44444h")

func loadScoresForItems(db *pg.DB, since time.Duration, key string) ([]app.Score, error) {
	par := make([]interface{}, 0)
	dumb := func(ups, downs int64) int64 {
		return ups - downs
	}
	keyClause := ""
	if len(key) > 0 {
		keyClause = `AND "items"."key" ~* ?0`
		par = append(par, interface{}(key))
	}
	scores := make([]app.Score, 0)
	q := fmt.Sprintf(`SELECT "items"."id", "items"."key", MAX("items"."submitted_at") AS "submitted_at",
		SUM(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
		SUM(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
		FROM "votes" INNER JOIN "items" ON "items"."id" = "votes"."item_id"
		WHERE "votes"."updated_at" >= current_timestamp - INTERVAL '%.3f hours' %s 
	GROUP BY "items"."id", "key" ORDER BY "items"."id";`,
		since.Hours(), keyClause)
	if _, err := db.Query(&scores, q, par...); err != nil {
		return nil, err
	} else {
		for k, score := range scores {
			var submitted time.Time

			now := time.Now().UTC()
			reddit := int64(app.Reddit(score.Ups, score.Downs, now.Sub(submitted)))
			wilson := int64(app.Wilson(score.Ups, score.Downs))
			hacker := int64(app.Hacker(score.Ups-score.Downs, now.Sub(submitted)))
			dumbScore := dumb(score.Ups, score.Downs)
			Logger.WithContext(log.Ctx{
				"key":    score.Key.String(),
				"ups":    score.Ups,
				"downs":  score.Downs,
				"reddit": reddit,
				"wilson": wilson,
				"hn":     hacker,
				"dumb":   dumbScore,
			}).Info("new score")

			score.Type = app.ScoreItem
			score.Score = dumbScore
			score.SubmittedAt = now
			scores[k] = score
		}
	}
	return scores, nil
}

func LoadScoresForAccounts(since time.Duration, col string, val string) ([]app.Score, error) {
	return loadScoresForAccounts(Config.DB, since, col, val)
}

func loadScoresForAccounts(db *pg.DB, since time.Duration, col string, val string) ([]app.Score, error) {
	par := make([]interface{}, 0)
	dumb := func(ups, downs int64) int64 {
		return ups - downs
	}
	keyClause := ""
	if len(val) > 0 && len(col) > 0 {
		keyClause = fmt.Sprintf(` and "items"."%s" ~* ?0`, col)
		par = append(par, interface{}(val))
	}
	scores := make([]app.Score, 0)
	q := fmt.Sprintf(`SELECT "accounts"."id", "accounts"."key", max("items"."submitted_at") as "submitted_at",
       SUM(CASE WHEN "weight" > 0 THEN "weight" ELSE 0 END) AS "ups",
       SUM(CASE WHEN "weight" < 0 THEN abs("weight") ELSE 0 END) AS "downs"
FROM "votes"
       INNER JOIN "items" ON "items"."id" = "item_id"
       INNER JOIN "accounts" ON "items"."submitted_by" = "accounts"."id"
WHERE "votes"."updated_at" >= current_timestamp - INTERVAL '%.3f hours' %s 
GROUP BY "accounts"."id", "accounts"."key" ORDER BY "accounts"."id";`,
		since.Hours(), keyClause)
	if _, err := db.Query(&scores, q, par...); err != nil {
		return nil, err
	} else {
		for _, score := range scores {
			var submitted time.Time

			now := time.Now().UTC()
			reddit := int64(app.Reddit(score.Ups, score.Downs, now.Sub(submitted)))
			wilson := int64(app.Wilson(score.Ups, score.Downs))
			hacker := int64(app.Hacker(score.Ups-score.Downs, now.Sub(submitted)))
			dumbScore := dumb(score.Ups, score.Downs)
			Logger.WithContext(log.Ctx{
				"key":    score.Key.String(),
				"ups":    score.Ups,
				"downs":  score.Downs,
				"reddit": reddit,
				"wilson": wilson,
				"hn":     hacker,
				"dumb":   dumbScore,
			}).Info("new score")

			score.Type = app.ScoreAccount
			score.Score = dumbScore
			score.SubmittedAt = now
		}
	}
	return scores, nil
}

// LoadInfo this method is here to keep compatibility with the repository interfaces
// but in the long term we might want to store some of this information in the DB
func (c config) LoadInfo() (app.Info, error) {
	return app.Instance.NodeInfo(), nil
}
