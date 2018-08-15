package models

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
)

const (
	FlagsDeleted = int8(1 << iota)

	FlagsNone    = int8(0)
)
const	MimeTypeURL  = "application/url"

type Key [64]byte

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

type item struct {
	Id          int64     `orm:id,"auto"`
	Key         Key       `orm:key,size(64)`
	Title       []byte    `orm:title`
	MimeType    string    `orm:mime_type`
	Data        []byte    `orm:data`
	Score       int64     `orm:score`
	SubmittedAt time.Time `orm:created_at`
	SubmittedBy int64     `orm:submitted_by`
	UpdatedAt   time.Time `orm:updated_at`
	Flags       int8      `orm:flags`
	Metadata    []byte    `orm:metadata`
	Path        []byte    `orm:path`
	SubmittedByAccount *Account
	fullPath    []byte
	parentLink  string
}

type ItemCollection []Item

func getAncestorHash(path []byte, cnt int) []byte {
	if path == nil {
		return nil
	}
	elem := bytes.Split(path, []byte("."))
	l := len(elem)
	if cnt > l || cnt < 0 {
		cnt = l
	}
	return elem[l-cnt]
}

func (c item) GetParentHash() string {
	if c.IsTop() {
		return ""
	}
	return string(getAncestorHash(c.Path, 1)[0:8])
}
func (c item) GetOPHash() string {
	if c.IsTop() {
		return ""
	}
	return string(getAncestorHash(c.Path, -1)[0:8])
}

func (c item) IsSelf() bool {
	mimeComponents := strings.Split(c.MimeType, "/")
	return mimeComponents[0] == "text"
}

func (c *item) GetKey() Key {
	data := c.Data
	now := c.UpdatedAt
	if now.IsZero() {
		now = time.Now()
	}
	data = append(data, []byte(fmt.Sprintf("%d", now.UnixNano()))...)
	data = append(data, []byte(c.Path)...)
	data = append(data, []byte(fmt.Sprintf("%d", c.SubmittedBy))...)

	c.Key.FromString(fmt.Sprintf("%x", sha256.Sum256(data)))
	return c.Key
}
func (c item) IsTop() bool {
	return c.Path == nil || len(c.Path) == 0
}
func (c item) Hash() string {
	return c.Hash8()
}
func (c item) Hash8() string {
	if len(c.Key) > 8 {
		return string(c.Key[0:8])
	}
	return c.Key.String()
}
func (c item) Hash16() string {
	if len(c.Key) > 16 {
		return string(c.Key[0:16])
	}
	return c.Key.String()
}
func (c item) Hash32() string {
	if len(c.Key) > 32 {
 	return string(c.Key[0:32])
	}
	return c.Key.String()
}
func (c item) Hash64() string {
	return c.Key.String()
}

func (c *item) FullPath() []byte {
	if len(c.fullPath) == 0 {
		c.fullPath = append(c.fullPath, c.Path...)
		if len(c.fullPath) > 0 {
			c.fullPath = append(c.fullPath, byte('.'))
		}
		c.fullPath = append(c.fullPath, c.Key.Bytes()...)
	}
	return c.fullPath
}

func (c item) Level() int {
	if c.Path == nil {
		return 0
	}
	return bytes.Count(c.FullPath(), []byte("."))
}
func (c item) Deleted() bool {
	return c.Flags & FlagsDeleted == FlagsDeleted
}
func (c item) UnDelete() {
	c.Flags ^= FlagsDeleted
}
func (c *item) Delete() {
	c.Flags &= FlagsDeleted
}
func (c item) IsLink() bool {
	return c.MimeType == MimeTypeURL
}
func (c item) GetDomain() string {
	if !c.IsLink() {
		return ""
	}
	return strings.Split(string(c.Data), "/")[2]
}

func LoadItems(filter LoadItemsFilter) (ItemCollection, error) {
	items := make(ItemCollection, 0)

	var wheres []string
	whereValues := make([]interface{}, 0)
	counter := 1
	if len(filter.SubmittedBy) > 0 {
		whereColumns := make([]string, 0)
		for _, v := range filter.SubmittedBy {
			whereColumns = append(whereColumns, fmt.Sprintf(`"accounts"."key" ~* $%d`, counter))
			whereValues = append(whereValues, interface{}(v))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR ")))
	}
	if len(filter.Type) > 0 {
		whereColumns := make([]string, 0)
		for _, typ := range filter.Type {
			if typ == TypeOP {
				whereColumns = append(whereColumns,`"content_items"."path" is NULL OR nlevel("content_items"."path") = 0`)
				counter += 1
			}
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	if len(filter.InReplyTo) > 0 {
		whereColumns := make([]string, 0)
		for _, hash := range filter.InReplyTo {
			whereColumns = append(whereColumns,fmt.Sprintf(`("content_items"."path" <@ (select
CASE WHEN path is null THEN key::ltree ELSE ltree_addltree(path, key::ltree) END
from "content_items" where key ~* $%d) AND "content_items"."path" IS NOT NULL)`, counter))
			whereValues = append(whereValues, interface{}(hash))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR "))))
	}
	if len(filter.Content) > 0 {
		whereColumns := make([]string, 0)
		var operator string
		if filter.ContentMatchType == MatchFuzzy {
			operator = "~"
		}
		if filter.ContentMatchType == MatchEquals{
			operator = "="
		}
		whereColumns = append(whereColumns, fmt.Sprintf(`"content_items"."data" %s $%d`, operator, counter))
		whereValues = append(whereValues, interface{}(filter.Content))
		counter += 1
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR ")))
	}
	if len(filter.MediaType) > 0 {
		whereColumns := make([]string, 0)
		for _, v := range filter.MediaType {
			whereColumns = append(whereColumns, fmt.Sprintf(`"content_items"."mime_type" = $%d`, counter))
			whereValues = append(whereValues, interface{}(v))
			counter += 1
		}
		wheres = append(wheres, fmt.Sprintf("(%s)", strings.Join(whereColumns, " OR ")))
	}
	var fullWhere string
	if len(wheres) > 0 {
		fullWhere = fmt.Sprintf(fmt.Sprintf("(%s)", strings.Join(wheres, " AND ")))
	} else {
		fullWhere = " true"
	}
	sel := fmt.Sprintf(`select 
			"content_items"."id", "content_items"."key", "content_items"."mime_type", "content_items"."data", 
			"content_items"."title", "content_items"."score", "content_items"."submitted_at", 
			"content_items"."submitted_by", "content_items"."flags", "content_items"."metadata", "content_items"."path",
			"accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score", 
			"accounts"."created_at", "accounts"."metadata", "accounts"."flags"
		from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by" 
		where %s 
	order by "content_items"."score" desc, "content_items"."submitted_at" desc limit %d`, fullWhere, filter.MaxItems)
	rows, err := Db.Query(sel, whereValues...)
	if err != nil {
		log.Error(errors.NewErrWithCause(err, "querying failed"))
		return nil, err
	}
	for rows.Next() {
		p := item{}
		a := account{}
		var aKey, iKey []byte
		err := rows.Scan(
			&p.Id, &iKey, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
			&a.Id, &aKey, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			log.Error(errors.NewErrWithCause(err, "load items failed"))
			continue
		}
		p.Key.FromBytes(iKey)
		a.Key.FromBytes(aKey)
		acct := loadAccountFromModel(a)
		p.SubmittedByAccount = &acct
		items = append(items, loadItemFromModel(p))
	}

	return items, nil
}

func LoadItem(f LoadItemsFilter) (Item, error) {
	if len(f.Key) == 0 {
		return Item{}, errors.Errorf("invalid item key to load")
	}

	hash := f.Key[0]
	p := item{}
	a := account{}
	i := Item{}
	var aKey, iKey []byte
	sel := `select "content_items"."id", "content_items"."key", "content_items"."mime_type", "content_items"."data", 
			"content_items"."title", "content_items"."score", "content_items"."submitted_at", 
			"content_items"."submitted_by", "content_items"."flags", "content_items"."metadata", "content_items"."path",
			"accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score", 
			"accounts"."created_at", "accounts"."metadata", "accounts"."flags"
 			from "content_items" 
			left join "accounts" on "accounts"."id" = "content_items"."submitted_by"
			where "content_items"."key" ~* $1`
	rows, err := Db.Query(sel, hash)
	if err != nil {
		return i, err
	}

	for rows.Next() {
		err = rows.Scan(&p.Id, &iKey, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
			&a.Id, &aKey, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return i, err
		}
		p.Key.FromBytes(iKey)
		a.Key.FromBytes(aKey)
		acct := loadAccountFromModel(a)
		p.SubmittedByAccount = &acct
	}
	i = loadItemFromModel(p)

	return i, nil
}

func LoadItemParent(hash string) (Item, error){
	i := Item{}
	p := item{}
	a := account{}

	sel := `select "par"."id", "par"."key", "par"."mime_type", "par"."data", 
			"par"."title", "par"."score", "par"."submitted_at", 
			"par"."submitted_by", "par"."flags", "par"."metadata", "par"."path",
			"accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score", 
			"accounts"."created_at", "accounts"."metadata", "accounts"."flags"
 			from "content_items" as "par"
			inner join "content_items" as "cur" on subltree("cur"."path", nlevel("cur"."path")-1, nlevel("cur"."path"))::text = "par"."key"
			left join "accounts" on "accounts"."id" = "par"."submitted_by"
			where "cur"."key" ~* $1`
	rows, err := Db.Query(sel, hash)
	if err != nil {
		return i, err
	}
	var aKey, iKey []byte
	for rows.Next() {
		err = rows.Scan(&p.Id, &iKey, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
			&a.Id, &aKey, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return i, err
		}
		p.Key.FromBytes(iKey)
		a.Key.FromBytes(aKey)
		acct := loadAccountFromModel(a)
		p.SubmittedByAccount = &acct
	}
	i = loadItemFromModel(p)
	return i, nil
}

func LoadItemOP(hash string) (Item, error){
	i := Item{}
	p := item{}
	a := account{}

	sel := `select "par"."id", "par"."key", "par"."mime_type", "par"."data",
       "par"."title", "par"."score", "par"."submitted_at",
       "par"."submitted_by", "par"."flags", "par"."metadata", "par"."path",
       "accounts"."id", "accounts"."key", "accounts"."handle", "accounts"."email", "accounts"."score",
       "accounts"."created_at", "accounts"."metadata", "accounts"."flags"
from "content_items" as "cur"
       inner join "content_items" as "par" on subltree("cur"."path", 0, 1)::text = "par"."key"
       left join "accounts" on "accounts"."id" = "par"."submitted_by"
			where "cur"."key" ~* $1`
	rows, err := Db.Query(sel, hash)
	if err != nil {
		return i, err
	}

	var aKey, iKey []byte
	for rows.Next() {
		err = rows.Scan(&p.Id, &iKey, &p.MimeType, &p.Data, &p.Title, &p.Score, &p.SubmittedAt, &p.SubmittedBy, &p.Flags, &p.Metadata, &p.Path,
			&a.Id, &aKey, &a.Handle, &a.Email, &a.Score, &a.CreatedAt, &a.Metadata, &a.Flags)
		if err != nil {
			return i, err
		}
		p.Key.FromBytes(iKey)
		a.Key.FromBytes(aKey)
		acct := loadAccountFromModel(a)
		p.SubmittedByAccount = &acct

		i = loadItemFromModel(p)
	}
	return i, nil
}
