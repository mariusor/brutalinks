package brutalinks

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"git.sr.ht/~mariusor/lw"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/filters/index"
	bolt "go.etcd.io/bbolt"
)

const (
	ByID                = index.ByID
	ByType              = index.ByType
	ByName              = index.ByName
	ByPreferredUsername = index.ByPreferredUsername
	BySummary           = index.BySummary
	ByContent           = index.ByContent
	ByActor             = index.ByActor
	ByObject            = index.ByObject
	ByRecipients        = index.ByRecipients
	ByAttributedTo      = index.ByAttributedTo
	ByInReplyTo         = index.ByInReplyTo
	ByPublished         = index.ByPublished
	ByUpdated           = index.ByUpdated
	ByCollection        = iota
)

type Storage struct {
	path string
	db   *bolt.DB
	in   map[index.Type]index.Indexable
	l    lw.Logger
}

var objectIndexTypes = []index.Type{
	ByID, ByType,
	ByRecipients, ByAttributedTo, ByInReplyTo,
	ByPublished, ByUpdated,
	ByName, BySummary, ByContent,
}

var actorIndexTypes = append(objectIndexTypes, ByPreferredUsername)

var activityIndexTypes = append(objectIndexTypes, ByActor, ByObject)

var collectionIndexTypes = []index.Type{ByCollection}

var allIndexTypes = append(append(append(objectIndexTypes, actorIndexTypes...), activityIndexTypes...), collectionIndexTypes...)

func NewStorage(path string) (*Storage, error) {
	s := new(Storage)
	s.path = path
	s.in = make(map[index.Type]index.Indexable)
	for _, typ := range allIndexTypes {
		switch typ {
		case ByID:
			// NOTE(marius): this is needed for actor/object byId filters to work
			s.in[typ] = index.All()
		case ByType:
			s.in[typ] = index.NewTokenIndex(index.ExtractType)
		case ByName:
			s.in[typ] = index.NewTokenIndex(index.ExtractName)
		case ByPreferredUsername:
			s.in[typ] = index.NewTokenIndex(index.ExtractPreferredUsername)
		case BySummary:
			s.in[typ] = index.NewTokenIndex(index.ExtractSummary)
		case ByContent:
			s.in[typ] = index.NewTokenIndex(index.ExtractContent)
		case ByActor:
			s.in[typ] = index.NewTokenIndex(index.ExtractActor)
		case ByObject:
			s.in[typ] = index.NewTokenIndex(index.ExtractObject)
		case ByRecipients:
			s.in[typ] = index.NewTokenIndex(index.ExtractRecipients)
		case ByAttributedTo:
			s.in[typ] = index.NewTokenIndex(index.ExtractAttributedTo)
		case ByInReplyTo:
			s.in[typ] = index.NewTokenIndex(index.ExtractInReplyTo)
		case ByCollection:
			s.in[typ] = index.NewIndex(index.ExtractCollectionItems, index.ExtractID)
		case ByPublished:
			s.in[typ] = index.NewIndex(index.ExtractPublished, index.ExtractID)
		case ByUpdated:
			s.in[typ] = index.NewIndex(index.ExtractUpdated, index.ExtractID)
		default:
		}
	}

	return s, nil
}

const defaultMode = 0600

func (st *Storage) Open() error {
	opts := bolt.DefaultOptions
	b, err := bolt.Open(st.path, defaultMode, opts)
	if err != nil {
		return err
	}
	st.db = b
	return st.loadIndexes()
}

func (st *Storage) Close() error {
	errs := make([]error, 0, 2)
	if !st.db.IsReadOnly() {
		if err := st.saveIndexes(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := st.db.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

var (
	indexBucketName = []byte("_index")

	idIndexName                = []byte("__id")
	typeIndexName              = []byte("__type")
	nameIndexName              = []byte("__name")
	preferredUsernameIndexName = []byte("__preferredUsername")
	summaryIndexName           = []byte("__summary")
	contentIndexName           = []byte("__content")
	actorIndexName             = []byte("__actor")
	objectIndexName            = []byte("__object")
	recipientsIndexName        = []byte("__recipients")
	attributedToIndexName      = []byte("__attributedTo")
	inReplyToIndexName         = []byte("__inReplyTo")
	collectionsIndexName       = []byte("__collections")
	publishedIndexName         = []byte("__published")
	updatedIndexName           = []byte("__updated")
)

func saveIndex(tx *bolt.Tx, name []byte, in index.Indexable) error {
	ib, err := tx.CreateBucketIfNotExists(indexBucketName)
	if err != nil {
		return err
	}

	buff := bytes.Buffer{}
	if err = gob.NewEncoder(&buff).Encode(in); err != nil {
		return err
	}

	return ib.Put(name, buff.Bytes())
}

func nameFromType(typ index.Type) []byte {
	switch typ {
	case ByID:
		return idIndexName
	case ByType:
		return typeIndexName
	case ByName:
		return nameIndexName
	case ByPreferredUsername:
		return preferredUsernameIndexName
	case BySummary:
		return summaryIndexName
	case ByContent:
		return contentIndexName
	case ByActor:
		return actorIndexName
	case ByObject:
		return objectIndexName
	case ByRecipients:
		return recipientsIndexName
	case ByAttributedTo:
		return attributedToIndexName
	case ByInReplyTo:
		return inReplyToIndexName
	case ByCollection:
		return collectionsIndexName
	case ByPublished:
		return publishedIndexName
	case ByUpdated:
		name := updatedIndexName
		return name
	}
	return nil
}

func loadIndex(tx *bolt.Tx, bmp index.Indexable, typ index.Type) error {
	ib := tx.Bucket(indexBucketName)
	if ib == nil {
		return nil
	}

	name := nameFromType(typ)
	if name == nil {
		return nil
	}

	raw := ib.Get(name)
	if raw == nil {
		return nil
	}

	return gob.NewDecoder(bytes.NewReader(raw)).Decode(bmp)
}

var (
	rootBucketName = []byte("_storage")

	itKey   = []byte("__raw")
	metaKey = []byte("__meta")
)

type metadata struct {
	Mod        time.Time
	Ref        uint64
	IRI        vocab.IRI
	UpdateTime time.Time
	// Top stores the top-most loaded item from a collection.
	Top vocab.IRI
	// Bottom stores the bottom-most loaded item from a collection.
	Bottom vocab.IRI
	// Exhausted marks that the collection has been fetched all the way to its beginning.
	Exhausted bool
}

func (st *Storage) itemExists(li vocab.LinkOrIRI) bool {
	exists := false
	iri := li.GetLink()

	// NOTE(marius): if an ActivityPub item arrives as a full object instead of an IRI reference,
	// we can assume it needs to be locally updated, unless it's some type of Activity which are
	// supposed to be immutable.
	if it, ok := li.(vocab.Item); ok {
		if !vocab.ActivityTypes.Match(it.GetType()) {
			return false
		}
	}

	_ = st.db.View(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket(rootBucketName)
		if b == nil {
			return fmt.Errorf("invalid root bucket %s", rootBucketName)
		}
		path := itemBucketPath(iri)
		if b, err = descendInBucket(b, path, false); err != nil {
			return err
		}
		exists = b.Get(itKey) != nil
		return nil
	})
	return exists
}

func (st *Storage) loadMetaData(iri vocab.IRI) (*metadata, error) {
	var m *metadata
	err := st.db.View(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket(rootBucketName)
		if b == nil {
			return fmt.Errorf("invalid root bucket %s", rootBucketName)
		}
		path := itemBucketPath(iri)
		if b, err = descendInBucket(b, path, false); err != nil {
			return err
		}
		mRaw := b.Get(metaKey)
		mm := metadata{}
		if err = gob.NewDecoder(bytes.NewReader(mRaw)).Decode(&mm); err != nil {
			return err
		}
		m = &mm
		return nil
	})

	return m, err
}

func refPath(x uint64) []byte {
	return []byte(strconv.FormatUint(x, 16))
}

func (st *Storage) loadFromRef(x uint64) (vocab.LinkOrIRI, error) {
	var it vocab.LinkOrIRI
	err := st.db.View(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket(rootBucketName)
		if b == nil {
			return fmt.Errorf("invalid root bucket %s", rootBucketName)
		}

		b = b.Bucket(refPath(x))
		if b == nil {
			return fmt.Errorf("invalid ref bucket %d", x)
		}

		raw := b.Get(itKey)
		it, err = vocab.UnmarshalJSON(raw)
		if err != nil {
			return err
		}
		mRaw := b.Get(metaKey)
		m := metadata{}
		if err = gob.NewDecoder(bytes.NewReader(mRaw)).Decode(&m); err != nil {
			return err
		}
		return nil
	})
	return it, err
}

func (st *Storage) Load(iri vocab.IRI) (vocab.LinkOrIRI, error) {
	var it vocab.LinkOrIRI
	err := st.db.View(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket(rootBucketName)
		if b == nil {
			return fmt.Errorf("invalid root bucket %s", rootBucketName)
		}
		path := itemBucketPath(iri)
		if b, err = descendInBucket(b, path, false); err != nil {
			return err
		}
		raw := b.Get(itKey)
		it, err = vocab.UnmarshalJSON(raw)
		if err != nil {
			return err
		}
		mRaw := b.Get(metaKey)
		m := metadata{}
		if err = gob.NewDecoder(bytes.NewReader(mRaw)).Decode(&m); err != nil {
			return err
		}
		return nil
	})
	return it, err
}

func (st *Storage) loadIndexes() error {
	return st.db.View(func(tx *bolt.Tx) error {
		errs := make([]error, 0)
		for typ := range st.in {
			if err := loadIndex(tx, st.in[typ], typ); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}

		return nil
	})
}

func (st *Storage) saveIndexes() error {
	return st.db.Update(func(tx *bolt.Tx) error {
		errs := make([]error, 0)
		for typ, bmp := range st.in {
			name := nameFromType(typ)
			if name == nil {
				return nil
			}
			if err := saveIndex(tx, name, bmp); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}

		return nil
	})
}

func (st *Storage) saveMetaData(it vocab.LinkOrIRI, m metadata) error {
	if it == nil {
		return nil
	}

	iri := it.GetLink()

	//if !st.itemExists(iri) {
	//	return nil
	//}

	buff := bytes.Buffer{}
	if err := gob.NewEncoder(&buff).Encode(m); err != nil {
		return err
	}

	err := st.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(rootBucketName)
		if b == nil {
			return fmt.Errorf("invalid root bucket %s", rootBucketName)
		}

		var err error
		path := itemBucketPath(iri)
		if b, err = descendInBucket(b, path, true); err != nil {
			return err
		}

		if err = b.Put(metaKey, buff.Bytes()); err != nil {
			return err
		}
		return nil
	})

	return err
}

func (st *Storage) AddIndex(it vocab.LinkOrIRI) error {
	for _, in := range st.in {
		in.Add(it)
	}
	return nil
}

func (st *Storage) Save(it vocab.LinkOrIRI) error {
	return st.save(it)
}

func (st *Storage) save(it vocab.LinkOrIRI) error {
	if it == nil || it.GetLink() == "" {
		return nil
	}
	if vocab.IsIRI(it) {
		return nil
	}

	raw, err := vocab.MarshalJSON(it)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil
	}

	iri := it.GetLink()

	m := metadata{IRI: iri, Ref: index.HashFn(iri)}
	switch ob := it.(type) {
	case vocab.Link:
		_ = st.AddIndex(ob)
		_ = vocab.OnLink(ob, func(l *vocab.Link) error {
			m.UpdateTime = time.Now().UTC()
			return nil
		})
	case vocab.Item:
		_ = st.AddIndex(ob)
		_ = vocab.OnObject(ob, func(o *vocab.Object) error {
			m.UpdateTime = time.Now().UTC()
			m.Mod = o.Published
			if o.Updated.After(m.Mod) {
				m.Mod = o.Updated
			}
			// TODO(marius): add object to replies collection of its replies/context members
			return nil
		})
		typ := ob.GetType()
		if vocab.ActivityTypes.Match(typ) {
			_ = st.in[ByActor].Add(ob)
			_ = st.in[ByObject].Add(ob)

			objectShouldUpdate := vocab.ActivityVocabularyTypes{vocab.CreateType, vocab.UpdateType, vocab.DeleteType}
			_ = vocab.OnActivity(ob, func(a *vocab.Activity) error {
				if !vocab.IsNil(a.Actor) && !st.itemExists(a.Actor) {
					_ = st.Save(a.Actor)
				}
				if !vocab.IsNil(a.Actor) && (!st.itemExists(a.Object) || objectShouldUpdate.Match(a.Type)) {
					_ = st.Save(a.Object)
				}
				return nil
			})
		}
		if vocab.IntransitiveActivityTypes.Match(typ) {
			_ = st.in[ByActor].Add(ob)
			_ = vocab.OnIntransitiveActivity(ob, func(a *vocab.IntransitiveActivity) error {
				if !vocab.IsNil(a.Actor) && !st.itemExists(a.Actor) {
					_ = st.Save(a.Actor)
				}
				return nil
			})
		}
	}

	buff := bytes.Buffer{}
	if err = gob.NewEncoder(&buff).Encode(m); err != nil {
		return err
	}

	err = st.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(rootBucketName)
		if err != nil {
			return err
		}
		path := itemBucketPath(it.GetLink())
		if b, err = descendInBucket(b, path, true); err != nil {
			return err
		}

		if err = b.Put(itKey, raw); err != nil {
			return err
		}

		if err = b.Put(metaKey, buff.Bytes()); err != nil {
			return err
		}
		return nil
	})

	return err
}

func itemBucketPath(iri vocab.IRI) []byte {
	ref := index.HashFn(iri)
	return refPath(ref)
}

var pathSeparator = []byte{os.PathSeparator}

func descendInBucket(root *bolt.Bucket, path []byte, editable bool) (*bolt.Bucket, error) {
	if root == nil {
		return nil, fmt.Errorf("trying to descend into nil bucket")
	}
	if len(path) == 0 {
		return root, nil
	}
	bucketNames := bytes.Split(bytes.TrimRight(path, string(pathSeparator)), pathSeparator)

	lvl := 0
	b := root
	// descend the bucket tree up to the last found bucket
	for _, name := range bucketNames {
		lvl++
		if len(name) == 0 {
			continue
		}
		var cb *bolt.Bucket
		if editable {
			var err error
			cb, err = b.CreateBucketIfNotExists(name)
			if err != nil {
				return nil, err
			}
		} else {
			cb = b.Bucket(name)
		}
		if cb == nil {
			lvl--
			break
		}
		b = cb
	}
	remBuckets := bucketNames[lvl:]
	path = bytes.Join(remBuckets, pathSeparator)
	return b, nil
}
