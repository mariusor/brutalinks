package models

import (
	"time"
	"database/sql"
	log "github.com/sirupsen/logrus"
	"fmt"
	"strings"
)

const (
	ScoreMultiplier = 1
	ScoreMaxK       = 10000.0
	ScoreMaxM       = 10000000.0
	ScoreMaxB       = 10000000000.0
)

type Vote struct {
	Id          int64     `orm:Id`
	SubmittedBy int64     `orm:submitted_by`
	SubmittedAt time.Time `orm:created_at`
	UpdatedAt   time.Time `orm:updated_at`
	ItemId      int64     `orm:item_id`
	Weight      int       `orm:Weight`
	Flags       int8      `orm:Flags`
	SubmittedByAccount *account
	Item 		*item
}

type Clauses []Clause
type Clause struct {
	ColName string
	Val interface{}
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
		placeHolders = append(placeHolders,"$1")
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
		placeHolders = append(placeHolders,"$1")
	}
	return strings.Join(placeHolders, " OR ")
}

func LoadVotes(db *sql.DB, wheres Clauses, max int) (*[]Vote, error) {
	var err error
	votes := make([]Vote, 0)
	selC := fmt.Sprintf(`select 
		"votes"."id", 
		"votes"."weight", 
		"votes"."submitted_at", 
		"votes"."flags",
		"content_items"."id", 
		"content_items"."key", 
		"content_items"."mime_type", 
		"content_items"."data", 
		"content_items"."title", 
		"content_items"."score",
		"content_items"."submitted_at", 
		"content_items"."submitted_by",
		"content_items"."flags", 
		"content_items"."metadata",
		"author"."id", "author"."key", "author"."handle", "author"."email", "author"."score", 
			"author"."created_at", "author"."metadata", "author"."flags",
		"voter"."id", "voter"."key", "voter"."handle", "voter"."email", "voter"."score", 
			"voter"."created_at", "voter"."metadata", "voter"."flags"
from "votes"
  inner join "content_items" on "content_items"."id" = "votes"."item_id"
  left join "accounts" as "author" on "author"."id" = "content_items"."submitted_by"
  left join "accounts" as "voter" on "voter"."id" = "votes"."submitted_by"
where %s order by "votes"."submitted_at" desc limit %d`, wheres.AndWhere(),  max)
	rows, err := db.Query(selC, wheres.Values()...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		v := Vote{}
		p := item{}
		auth := account{}
		voter := account{}
		var pKey []byte
		var aKey []byte
		var vKey []byte
		err = rows.Scan(
			&v.Id,
			&v.Weight,
			&v.SubmittedAt,
			&v.Flags,
			&p.Id,
			&pKey,
			&p.MimeType,
			&p.Data,
			&p.Title,
			&p.Score,
			&p.SubmittedAt,
			&p.SubmittedBy,
			&p.Flags,
			&p.Metadata,
			&auth.Id, &aKey, &auth.Handle, &auth.Email, &auth.Score, &auth.CreatedAt, &auth.Metadata, &auth.Flags,
			&voter.Id, &vKey, &voter.Handle, &voter.Email, &voter.Score, &voter.CreatedAt, &voter.Metadata, &voter.Flags)
		if err != nil {
			return nil, err
		}
		auth.Key.FromBytes(aKey)
		acct := loadFromModel(auth)
		p.SubmittedByAccount = &acct
		p.Key.FromBytes(pKey)

		voter.Key.FromBytes(vKey)
		v.SubmittedByAccount = &voter
		v.Item = &p

		votes = append(votes, v)
	}
	if err != nil {
		log.Error(err)
	}
	return &votes, nil
}

func LoadItemsVotes(db *sql.DB, hash string) (*item, *[]Vote, error) {
	var err error
	p := item{}
	votes := make([]Vote, 0)
	selC := `select 
		"votes"."id", 
		"votes"."weight", 
		"votes"."submitted_at", 
		"votes"."flags",
		"content_items"."id", 
		"content_items"."key", 
		"content_items"."mime_type", 
		"content_items"."data", 
		"content_items"."title", 
		"content_items"."score",
		"content_items"."submitted_at", 
		"content_items"."submitted_by",
		"content_items"."flags", 
		"content_items"."metadata", 
		"accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score", 
			"accounts"."created_at", "accounts"."metadata", "accounts"."flags"
from "content_items"
  inner join "votes" on "content_items"."id" = "votes"."item_id"
  left join "accounts" on "accounts"."id" = "content_items"."submitted_by"
where "content_items"."key" ~* $1 order by "votes"."submitted_at" desc`
	{
		rows, err := db.Query(selC, hash)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			v := Vote{}
			a := account{}
			err = rows.Scan(
				&v.Id,
				&v.Weight,
				&v.SubmittedAt,
				&v.Flags,
				&p.Id,
				&p.Key,
				&p.MimeType,
				&p.Data,
				&p.Title,
				&p.Score,
				&p.SubmittedAt,
				&p.SubmittedBy,
				&p.Flags,
				&p.Metadata,
				&a.Id, &a.Key, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
			if err != nil {
				return nil, nil, err
			}
			acct := loadFromModel(a)
			p.SubmittedByAccount = &acct
			votes = append(votes, v)
		}
	}
	if err != nil {
		log.Error(err)
	}
	return &p, &votes, nil
}

func LoadVotesSubmittedBy(db *sql.DB, handle string, which int, max int) (*[]Vote, error) {
	clauses := make(Clauses, 0)
	clauses = append(clauses, Clause{ColName: `"voter"."handle" = `, Val: interface{}(handle)})
	if which != 0 {
		if which > 0 {
			clauses = append(clauses, Clause{ColName: `"votes"."weight" > `, Val: interface{}(0)})
		} else {
			clauses = append(clauses, Clause{ColName: `"votes"."weight" < `, Val: interface{}(0)})
		}
	}
	return LoadVotes(db, clauses, max)
}

type ScoreType int
const (
	ScoreItem = ScoreType(iota)
	ScoreAccount
)
type Score struct {
	Id  int64
	Key []byte
	Score int64
	Submitted time.Time
	Type ScoreType
}

func LoadScoresForItems(db *sql.DB, since time.Duration, key string) ([]Score, error) {
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
		log.Infof("Votes[%s]: UPS[%d] DOWNS[%d] - new score %d:%d:%d", key, ups, downs, reddit, wilson, hacker)
		new := Score{
			Id: i,
			Key: key,
			Submitted: submitted,
			Type: ScoreAccount,
			Score: hacker,
		}
		scores = append(scores, new)
	}
	return scores, nil
}

func LoadScoresForAccounts(db *sql.DB, since time.Duration, col string, val string)  ([]Score, error) {
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
		log.Infof("Votes[%s]: UPS[%d] DOWNS[%d] - new score %d:%d:%d", handle, ups, downs, reddit, wilson, hacker)
		new := Score{
			Id: i,
			Key: key,
			Submitted: submitted,
			Type: ScoreAccount,
			Score: hacker,
		}
		scores = append(scores, new)
	}
	return scores, nil
}
