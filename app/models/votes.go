package models

import (
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
)

const (
	ScoreMultiplier = 1
	ScoreMaxK       = 10000.0
	ScoreMaxM       = 10000000.0
	ScoreMaxB       = 10000000000.0
)

type VoteCollection map[Hash]Vote

type Vote struct {
	SubmittedBy *Account  `json:"submittedBy"`
	SubmittedAt time.Time `json:"-"`
	UpdatedAt   time.Time `json:"-"`
	Weight      int       `json:"weight"`
	Item        *Item     `json:"-"`
	Flags       FlagBits  `json:"-"`
}

type vote struct {
	Id                 int64     `orm:id`
	SubmittedBy        int64     `orm:submitted_by`
	SubmittedAt        time.Time `orm:created_at`
	UpdatedAt          time.Time `orm:updated_at`
	ItemId             int64     `orm:item_id`
	Weight             int       `orm:weight`
	Flags              FlagBits  `orm:flags`
	SubmittedByAccount *account
	Item               *item
}

func loadVoteFromModel(v vote, a *account, i *item) Vote {
	vv := Vote{
		SubmittedAt: v.SubmittedAt,
		UpdatedAt:   v.UpdatedAt,
		Weight:      v.Weight,
		Flags:       v.Flags,
	}
	if i != nil {
		it := loadItemFromModel(*i)
		vv.Item = &it
	}
	if a != nil {
		act := loadAccountFromModel(*a)
		vv.SubmittedBy = &act
	}
	return vv
}

type Clauses []Clause
type Clause struct {
	ColName string
	Val     interface{}
}

func (cl Clauses) Values() []interface{} {
	clauses := make([]interface{}, 0)
	if cl == nil || len(cl) == 0 {
		clauses = append(clauses, interface{}(true))
	} else {
		for _, t := range cl {
			clauses = append(clauses, t.Val)
		}
	}
	return clauses
}
func (cl Clauses) AndWhere() string {
	placeHolders := make([]string, 0)
	if cl == nil || len(cl) == 0 {
		placeHolders = append(placeHolders, "$1")
	} else {
		for i, t := range cl {
			placeHolders = append(placeHolders, fmt.Sprintf("%s $%d", t.ColName, i+1))
		}
	}
	return strings.Join(placeHolders, " AND ")
}

func (cl Clauses) OrWhere() string {
	placeHolders := make([]string, 0)
	if cl == nil || len(cl) == 0 {
		placeHolders = append(placeHolders, "$1")
	}
	return strings.Join(placeHolders, " OR ")
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

func loadVotes(db *sql.DB, filter LoadVotesFilter) (VoteCollection, error) {
	var err error
	votes := make(VoteCollection, 0)

	var wheres []string
	whereValues := make([]interface{}, 0)
	counter := 1
	if len(filter.AttributedTo) > 0 {
		whereColumns := make([]string, 0)
		for _, v := range filter.AttributedTo {
			whereColumns = append(whereColumns, fmt.Sprintf(`"voter"."key" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(v))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR ")))
	}
	if len(filter.Type) > 0 {
		whereColumns := make([]string, 0)
		for _, typ := range filter.Type {
			if typ == TypeLike {
				whereColumns = append(whereColumns, fmt.Sprintf(`"votes"."weight" > $%d`, counter))
				whereValues = append(whereValues, interface{}(0))
			}
			if typ == TypeDislike {
				whereColumns = append(whereColumns, fmt.Sprintf(`"votes"."weight" < $%d`, counter))
				whereValues = append(whereValues, interface{}(0))
			}
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	if len(filter.ItemKey) > 0 {
		whereColumns := make([]string, 0)
		for _, k := range filter.ItemKey {
			h := trimHash(k)
			if len(h) == 0 {
				continue
			}
			whereColumns = append(whereColumns, fmt.Sprintf(`"items"."key" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(h))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	fullWhere := fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(wheres, " AND ")))

	selC := fmt.Sprintf(`select
		  "votes"."id", "votes"."weight", "votes"."submitted_at", "votes"."flags",
       "items"."id", "items"."key", "items"."mime_type", "items"."data", "items"."title", "items"."score",
       "items"."submitted_at", "items"."submitted_by", "items"."flags", "items"."metadata",
       "voter"."id", "voter"."key", "voter"."handle", "voter"."email", "voter"."score",
       "voter"."created_at", "voter"."metadata", "voter"."flags",
       "author"."id", "author"."key", "author"."handle", "author"."email", "author"."score",
       "author"."created_at", "author"."metadata", "author"."flags"
from "votes"
       inner join "accounts" as "voter" on "voter"."id" = "votes"."submitted_by"
       inner join "content_items" as "items" on "items"."id" = "votes"."item_id"
       left join "accounts" as "author" on "author"."id" = "items"."submitted_by"
where %s order by "votes"."submitted_at" desc limit %d`, fullWhere, filter.MaxItems)
	rows, err := db.Query(selC, whereValues...)
	if err != nil {
		log.WithFields(log.Fields{}).Error(errors.NewErrWithCause(err, "querying failed"))
		return nil, err
	}
	for rows.Next() {
		v := vote{}
		p := item{}
		voter := account{}
		auth := account{}
		var pKey []byte
		var aKey []byte
		var vKey []byte
		err = rows.Scan(&v.Id, &v.Weight, &v.SubmittedAt, &v.Flags,
			&p.Id, &pKey, &p.MimeType, &p.Data, &p.Title, &p.Score,
			&p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata,
			&voter.Id, &aKey, &voter.Handle, &voter.Email, &voter.Score, &voter.CreatedAt, &voter.Metadata, &voter.Flags,
			&auth.Id, &vKey, &auth.Handle, &auth.Email, &auth.Score, &auth.CreatedAt, &auth.Metadata, &auth.Flags)
		if err != nil {
			log.WithFields(log.Fields{}).Errorf("load items failed: %s", err.Error())
			continue
		}
		voter.Key.FromBytes(aKey)
		acct := loadAccountFromModel(auth)
		p.SubmittedByAccount = &acct
		p.Key.FromBytes(pKey)

		auth.Key.FromBytes(vKey)
		v.Item = &p
		votes[p.Hash64()] = loadVoteFromModel(v, &voter, &p)
	}
	if err != nil {
		log.WithFields(log.Fields{}).Errorf("load votes failed: %s", err.Error())
		return nil, err
	}
	return votes, nil
}

type ScoreType int

const (
	ScoreItem = ScoreType(iota)
	ScoreAccount
)

type Score struct {
	Id        int64
	Key       []byte
	Score     int64
	Submitted time.Time
	Type      ScoreType
}

func LoadScoresForItems(since time.Duration, key string) ([]Score, error) {
	return loadScoresForItems(Config.DB, since, key)
}

func loadScoresForItems(db *sql.DB, since time.Duration, key string) ([]Score, error) {
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
	scores := make([]Score, 0)
	for rows.Next() {
		var i, ups, downs int64
		var submitted time.Time
		var key []byte
		err = rows.Scan(&i, &key, &submitted, &ups, &downs)

		now := time.Now()
		reddit := int64(Reddit(ups, downs, now.Sub(submitted)))
		wilson := int64(Wilson(ups, downs))
		hacker := int64(Hacker(ups-downs, now.Sub(submitted)))
		log.WithFields(log.Fields{}).Infof("Votes[%s]: UPS[%d] DOWNS[%d] - new score %d:%d:%d", key, ups, downs, reddit, wilson, hacker)
		new := Score{
			Id:        i,
			Key:       key,
			Submitted: submitted,
			Type:      ScoreAccount,
			Score:     hacker,
		}
		scores = append(scores, new)
	}
	return scores, nil
}

func LoadScoresForAccounts(since time.Duration, col string, val string) ([]Score, error) {
	return loadScoresForAccounts(Config.DB, since, col, val)
}

func loadScoresForAccounts(db *sql.DB, since time.Duration, col string, val string) ([]Score, error) {
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

	scores := make([]Score, 0)
	for rows.Next() {
		var i, ups, downs int64
		var submitted time.Time
		var key []byte
		var handle string
		err = rows.Scan(&i, &handle, &key, &submitted, &ups, &downs)

		now := time.Now()
		reddit := int64(Reddit(ups, downs, now.Sub(submitted)))
		wilson := int64(Wilson(ups, downs))
		hacker := int64(Hacker(ups-downs, now.Sub(submitted)))
		log.WithFields(log.Fields{}).Infof("Votes[%s]: UPS[%d] DOWNS[%d] - new score %d:%d:%d", handle, ups, downs, reddit, wilson, hacker)
		new := Score{
			Id:        i,
			Key:       key,
			Submitted: submitted,
			Type:      ScoreAccount,
			Score:     hacker,
		}
		scores = append(scores, new)
	}
	return scores, nil
}

func AddVote(p Item, score int, userHash string) (bool, error) {
	return addVote(Config.DB, p, score, userHash)
}

// AddVote adds a vote to the p content item
//   const {
//      add_vote = "add_vote"
//      delete = "delete"
//   }
//   type queue_message struct {
//       type    string
//       payload json.RawMessage
//   }
// Ideally this should be done asynchronously pushing an add_vote message to our
// messaging queue. Details of this queue to be established (strongest possibility is Redis PubSub)
// The cli/votes/main.go script would be responsible with waiting on the queue for these messages
// and updating the new score and all models dependent on it.
//   content_items and accounts tables, corresponding ES documents, etc
func addVote(db *sql.DB, p Item, score int, userHash string) (bool, error) {
	newWeight := int(score * ScoreMultiplier)

	var sel string
	var p2 interface{}
	//if p.Id == 0 {
	sel = `select "id", "accounts"."id", "weight" from "votes" 
		inner join "accounts" on "accounts"."id" = "votes"."submitted_by" where "accounts"."hash" ~* $1 and "key" ~* $2;`
	p2 = interface{}(p.Hash)
	//} else {
	//	sel = `select "id", "weight" from "votes" where "submitted_by" = $1 and "item_id" = $2;`
	//	p2 = interface{}(p.Id)
	//}

	var userId int64
	var vId int64
	v := Vote{}
	{
		rows, err := db.Query(sel, userHash, p2)
		if err != nil {
			return false, err
		}
		for rows.Next() {
			err = rows.Scan(&vId, &userId, &v.Weight)
			if err != nil {
				return false, err
			}
		}
	}

	oldWeight := v.Weight
	var q string
	if vId != 0 {
		if v.Weight != 0 && math.Signbit(float64(newWeight)) == math.Signbit(float64(v.Weight)) {
			newWeight = 0
		}
		q = `update "votes" set "updated_at" = now(), "weight" = $1 where "item_id" = $2 and "submitted_by" = $3;`
	} else {
		q = `insert into "votes" ("weight", "item_id", "submitted_by") values ($1, $2, $3)`
	}
	{
		res, err := db.Exec(q, newWeight, p.Hash, userId)
		if err != nil {
			return false, err
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			return false, errors.Errorf("scoring %d failed on item %q", newWeight, p.Hash)
		}
		log.WithFields(log.Fields{}).Infof("%d scoring %d on %s", userId, newWeight, p.Hash)
	}

	upd := `update "content_items" set score = score - $1 + $2 where "id" = $3`
	{
		res, err := db.Exec(upd, v.Weight, newWeight, p.Hash)
		if err != nil {
			return false, err
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			return false, errors.Errorf("content hash %q not found", p.Hash)
		}
		if rows, _ := res.RowsAffected(); rows > 1 {
			return false, errors.Errorf("content hash %q collision", p.Hash)
		}
		log.WithFields(log.Fields{
			"hash":      p.Hash,
			"newWeight": newWeight,
			"oldWeight": oldWeight,
		}).Infof("updated item")
	}

	return true, nil
}

func (v VoteCollection) First() (*Vote, error) {
	for _, vv := range v {
		return &vv, nil
	}
	return nil, errors.Errorf("empty %T", v)
}
