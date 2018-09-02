package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	j "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
)

var Db *sql.DB

const (
	MaxContentItems = 200
)

var BaseURL string
var AccountsURL string
var OutboxURL string

const NotFound = 404
const InternalError = 500

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Fields []Field

type Error struct {
	Code  int
	Error error
}

func init() {
	https := os.Getenv("HTTPS") != ""
	host := os.Getenv("HOSTNAME")

	if https {
		BaseURL = fmt.Sprintf("https://%s/api", host)
	} else {
		BaseURL = fmt.Sprintf("http://%s/api", host)
	}

	AccountsURL = BaseURL + "/accounts"
	OutboxURL = BaseURL + "/outbox"
}

type (
	ObjectID               ap.ObjectID
	ActivityVocabularyType ap.ActivityVocabularyType
	NaturalLanguageValue   ap.NaturalLanguageValue
	ObjectOrLink           ap.ObjectOrLink
	LinkOrURI              ap.LinkOrURI
	ImageOrLink            ap.ImageOrLink
	MimeType               ap.MimeType
	ObjectsArr             ap.ObjectsArr
	CollectionInterface    ap.CollectionInterface
	Endpoints              ap.Endpoints
)

type PublicKey struct {
	Id           ObjectID     `jsonld:"id,omitempty"`
	Owner        ObjectOrLink `jsonld:"owner,omitempty"`
	PublicKeyPem string       `jsonld:"publicKeyPem,omitempty"`
}

// Person it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/actors.go#Actor
// We need it here in order to be able to add to it our Score property
type Person struct {
	ID                ObjectID               `jsonld:"id,omitempty"`
	Type              ActivityVocabularyType `jsonld:"type,omitempty"`
	Name              ap.NaturalLanguageValue   `jsonld:"name,omitempty,collapsible"`
	Attachment        ObjectOrLink           `jsonld:"attachment,omitempty"`
	AttributedTo      ObjectOrLink           `jsonld:"attributedTo,omitempty"`
	Audience          ObjectOrLink           `jsonld:"audience,omitempty"`
	Content           ap.NaturalLanguageValue   `jsonld:"content,omitempty,collapsible"`
	Context           ObjectOrLink           `jsonld:"context,omitempty,collapsible"`
	EndTime           time.Time              `jsonld:"endTime,omitempty"`
	Generator         ObjectOrLink           `jsonld:"generator,omitempty"`
	Icon              ImageOrLink            `jsonld:"icon,omitempty"`
	Image             ImageOrLink            `jsonld:"image,omitempty"`
	InReplyTo         ObjectOrLink           `jsonld:"inReplyTo,omitempty"`
	Location          ObjectOrLink           `jsonld:"location,omitempty"`
	Preview           ObjectOrLink           `jsonld:"preview,omitempty"`
	Published         time.Time              `jsonld:"published,omitempty"`
	Replies           CollectionInterface    `jsonld:"replies,omitempty"`
	StartTime         time.Time              `jsonld:"startTime,omitempty"`
	Summary           ap.NaturalLanguageValue   `jsonld:"summary,omitempty,collapsible"`
	Tag               ObjectOrLink           `jsonld:"tag,omitempty"`
	Updated           time.Time              `jsonld:"updated,omitempty"`
	URL               LinkOrURI              `jsonld:"url,omitempty"`
	To                ObjectsArr             `jsonld:"to,omitempty"`
	Bto               ObjectsArr             `jsonld:"bto,omitempty"`
	CC                ObjectsArr             `jsonld:"cc,omitempty"`
	BCC               ObjectsArr             `jsonld:"bcc,omitempty"`
	Duration          time.Duration          `jsonld:"duration,omitempty"`
	Inbox             CollectionInterface    `jsonld:"inbox,omitempty"`
	Outbox            CollectionInterface    `jsonld:"outbox,omitempty"`
	Following         CollectionInterface    `jsonld:"following,omitempty"`
	Followers         CollectionInterface    `jsonld:"followers,omitempty"`
	Liked             CollectionInterface    `jsonld:"liked,omitempty"`
	PreferredUsername ap.NaturalLanguageValue   `jsonld:"preferredUsername,omitempty,collapsible"`
	Endpoints         Endpoints              `jsonld:"endpoints,omitempty"`
	Streams           []CollectionInterface  `jsonld:"streams,omitempty"`
	PublicKey         PublicKey              `jsonld:"publicKey,omitempty"`
	// Score is our own custom property for which we needed to extend the existing AP one
	Score int64 `jsonld:"score"`
}

func (p Person) GetID() *ap.ObjectID {
	id := ap.ObjectID(p.ID)
	return &id
}
func (p Person) GetType() ap.ActivityVocabularyType {
	return ap.ActivityVocabularyType(p.Type)
}
func (p Person) IsLink() bool {
	return false
}
func (p Person) IsObject() bool {
	return true
}

// Article it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/objects.go#Object
// We need it here in order to be able to add to it our Score property
type Article struct {
	ID           ObjectID                `jsonld:"id,omitempty"`
	Type         ActivityVocabularyType  `jsonld:"type,omitempty"`
	Name         ap.NaturalLanguageValue `jsonld:"name,omitempty,collapsible"`
	Attachment   ObjectOrLink            `jsonld:"attachment,omitempty"`
	AttributedTo ObjectOrLink            `jsonld:"attributedTo,omitempty"`
	Audience     ObjectOrLink            `jsonld:"audience,omitempty"`
	Content      ap.NaturalLanguageValue `jsonld:"content,omitempty,collapsible"`
	Context      ObjectOrLink            `jsonld:"context,omitempty"`
	MediaType    MimeType                `jsonld:"mediaType,omitempty"`
	EndTime      time.Time               `jsonld:"endTime,omitempty"`
	Generator    ObjectOrLink            `jsonld:"generator,omitempty"`
	Icon         ImageOrLink             `jsonld:"icon,omitempty"`
	Image        ImageOrLink             `jsonld:"image,omitempty"`
	InReplyTo    ObjectOrLink            `jsonld:"inReplyTo,omitempty"`
	Location     ObjectOrLink            `jsonld:"location,omitempty"`
	Preview      ObjectOrLink            `jsonld:"preview,omitempty"`
	Published    time.Time               `jsonld:"published,omitempty"`
	Replies      CollectionInterface     `jsonld:"replies,omitempty"`
	StartTime    time.Time               `jsonld:"startTime,omitempty"`
	Summary      NaturalLanguageValue    `jsonld:"summary,omitempty,collapsible"`
	Tag          ObjectOrLink            `jsonld:"tag,omitempty"`
	Updated      time.Time               `jsonld:"updated,omitempty"`
	URL          LinkOrURI               `jsonld:"url,omitempty"`
	To           ObjectsArr              `jsonld:"to,omitempty"`
	Bto          ObjectsArr              `jsonld:"bto,omitempty"`
	CC           ObjectsArr              `jsonld:"cc,omitempty"`
	BCC          ObjectsArr              `jsonld:"bcc,omitempty"`
	Duration     time.Duration           `jsonld:"duration,omitempty"`
	// Score is our own custom property for which we needed to extend the existing AP one
	Score int64 `jsonld:"score"`
}

func (a Article) GetID() *ap.ObjectID {
	id := ap.ObjectID(a.ID)
	return &id
}
func (a Article) GetType() ap.ActivityVocabularyType {
	return ap.ActivityVocabularyType(a.Type)
}
func (a Article) IsLink() bool {
	return false
}
func (a Article) IsObject() bool {
	return true
}

// OrderedCollection it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/collections.go#OrderedCollection
// We need it here in order to be able to implement our own UnmarshalJSON() method
type OrderedCollection struct {
	ID           ObjectID               `jsonld:"id,omitempty"`
	Type         ActivityVocabularyType `jsonld:"type,omitempty"`
	Name         NaturalLanguageValue   `jsonld:"name,omitempty,collapsible"`
	Attachment   ObjectOrLink           `jsonld:"attachment,omitempty"`
	AttributedTo ObjectOrLink           `jsonld:"attributedTo,omitempty"`
	Audience     ObjectOrLink           `jsonld:"audience,omitempty"`
	Content      NaturalLanguageValue   `jsonld:"content,omitempty,collapsible"`
	Context      ObjectOrLink           `jsonld:"context,omitempty,collapsible"`
	EndTime      time.Time              `jsonld:"endTime,omitempty"`
	Generator    ObjectOrLink           `jsonld:"generator,omitempty"`
	InReplyTo    ObjectOrLink           `jsonld:"inReplyTo,omitempty"`
	Location     ObjectOrLink           `jsonld:"location,omitempty"`
	Preview      ObjectOrLink           `jsonld:"preview,omitempty"`
	Published    time.Time              `jsonld:"published,omitempty"`
	Replies      CollectionInterface    `jsonld:"replies,omitempty"`
	Summary      NaturalLanguageValue   `jsonld:"summary,omitempty,collapsible"`
	Tag          ObjectOrLink           `jsonld:"tag,omitempty"`
	Updated      time.Time              `jsonld:"updated,omitempty"`
	URL          LinkOrURI              `jsonld:"url,omitempty"`
	Duration     time.Duration          `jsonld:"duration,omitempty"`
	TotalItems   uint                   `jsonld:"totalItems,omitempty"`
	OrderedItems []Article              `jsonld:"orderedItems,omitempty"`
}

func (a *Article) UnmarshalJSON(data []byte) error {
	it := ap.Object{}
	err := it.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	a.ID = ObjectID(*it.GetID())
	a.Type = ActivityVocabularyType(it.GetType())
	a.Name = it.Name
	a.Content = it.Content
	a.Context = it.Context
	a.Generator = it.Generator
	a.AttributedTo = it.AttributedTo
	a.Published = it.Published
	a.MediaType = MimeType(it.MediaType)
	if score, err := jsonparser.GetInt(data, "score"); err == nil {
		a.Score = score
	}
	if inReplyTo, err := jsonparser.GetString(data, "inReplyTo"); err == nil {
		a.InReplyTo = ap.IRI(inReplyTo)
	}
	if context, err := jsonparser.GetString(data, "context"); err == nil {
		a.Context = ap.IRI(context)
	}

	return nil
}

func (o *OrderedCollection) UnmarshalJSON(data []byte) error {
	col := ap.OrderedCollection{}
	err := col.UnmarshalJSON(data)
	if err != nil {
		return err
	}
	o.ID = ObjectID(col.ID)
	o.Type = ActivityVocabularyType(col.Type)
	o.TotalItems = col.TotalItems
	o.OrderedItems = make([]Article, o.TotalItems)
	for i := range col.OrderedItems {
		a := Article{}
		if data, _, _, err := jsonparser.Get(data, "orderedItems", fmt.Sprintf("[%d]", i)); err == nil {
			a.UnmarshalJSON(data)
		}
		if context, err := jsonparser.GetString(data, "orderedItems", fmt.Sprintf("[%d]", i), "context"); err == nil {
			a.Context = ap.IRI(context)
		}
		o.OrderedItems[i] = a
	}
	return nil
}

func (p *Person) UnmarshalJSON(data []byte) error {
	app := ap.Person{}
	err := app.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	p.ID = ObjectID(*app.GetID())
	p.Type = ActivityVocabularyType(app.GetType())
	p.Name = app.Name
	p.Content = app.Content
	p.Context = app.Context
	p.Generator = app.Generator
	p.AttributedTo = app.AttributedTo
	p.Published = app.Published
	if score, err := jsonparser.GetInt(data, "score"); err == nil {
		p.Score = score
	}
	if inReplyTo, err := jsonparser.GetString(data, "inReplyTo"); err == nil {
		p.InReplyTo = ap.IRI(inReplyTo)
	}
	if context, err := jsonparser.GetString(data, "context"); err == nil {
		p.Context = ap.IRI(context)
	}
	if outbox, _, _, err := jsonparser.Get(data, "outbox"); err == nil {
		c := ap.OrderedCollection{}
		j.Unmarshal(outbox, &c)
		p.Outbox = &c
	}
	if inbox, _, _, err := jsonparser.Get(data, "inbox"); err == nil {
		c := ap.OrderedCollection{}
		j.Unmarshal(inbox, &c)
		p.Inbox = &c
	}
	if liked, _, _, err := jsonparser.Get(data, "liked"); err == nil {
		c := ap.OrderedCollection{}
		j.Unmarshal(liked, &c)
		p.Liked = &c
	}
	if replies, _, _, err := jsonparser.Get(data, "replies"); err == nil {
		c := ap.Collection{}
		j.Unmarshal(replies, &c)
		p.Replies = &c
	}
	return nil
}

func getHash(i *ap.ObjectID) string {
	if i == nil {
		return ""
	}
	s := strings.Split(string(*i), "/")
	return s[len(s)-1]
}

func getAccountHandle(o ObjectOrLink) string {
	if o == nil {
		return ""
	}
	i := o.(ap.IRI)
	s := strings.Split(string(i), "/")
	return s[len(s)-1]
}

func Errorf(c int, m string, args ...interface{}) *Error {
	return &Error{c, errors.Errorf(m, args...)}
}

func GetContext() j.Context {
	return j.Context{
		j.Term(j.NilTerm): j.IRI(ap.ActivityBaseURI),
		j.Term("w3id"):    j.IRI("https://w3id.org/security/v1"),
		j.Term("score"):   j.IRI("http://littr.me/as#score"),
	}
}

func BuildActorID(a models.Account) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/%s", AccountsURL, url.PathEscape(a.Handle)))
}

func BuildCollectionID(a models.Account, o ap.CollectionInterface) ap.ObjectID {
	if len(a.Handle) > 0 {
		return ap.ObjectID(fmt.Sprintf("%s/%s/%s", AccountsURL, url.PathEscape(a.Handle), getObjectType(o)))
	}
	return ap.ObjectID(fmt.Sprintf("%s/%s", BaseURL, getObjectType(o)))
}

func BuildRepliesCollectionID(i ap.Item) ap.ObjectID {
	return ap.ObjectID(fmt.Sprintf("%s/replies", *i.GetID()))
}

func BuildObjectIDFromItem(i models.Item) ap.ObjectID {
	handle := "anonymous"
	if i.SubmittedBy != nil {
		handle = i.SubmittedBy.Handle
	}
	return ap.ObjectID(fmt.Sprintf("%s/%s/outbox/%s", AccountsURL, url.PathEscape(handle), url.PathEscape(i.Hash)))
}

func BuildObjectIDFromVote(v models.Vote) ap.ObjectID {
	att := "liked"
	//if v.Weight < 0 {
	//	att = "disliked"
	//}
	return ap.ObjectID(fmt.Sprintf("%s/%s/%s/%s", AccountsURL, url.PathEscape(v.SubmittedBy.Handle), att, url.PathEscape(v.Item.Hash)))
}

func getObjectType(el ap.Item) string {
	if el == nil {
		return ""
	}
	var (
		label               = ""
		typeOutbox          = reflect.TypeOf(ap.Outbox{})
		typeOutboxStream    = reflect.TypeOf(ap.OutboxStream{})
		typeInbox           = reflect.TypeOf(ap.Inbox{})
		typeInboxStream     = reflect.TypeOf(ap.InboxStream{})
		typeLiked           = reflect.TypeOf(ap.Liked{})
		typeLikedCollection = reflect.TypeOf(ap.LikedCollection{})
		typePerson          = reflect.TypeOf(ap.Person{})
		typeLocalPerson     = reflect.TypeOf(Person{})
	)
	typ := reflect.TypeOf(el)
	val := reflect.ValueOf(el)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		val = val.Elem()
	}
	switch typ {
	case typeOutbox:
		fallthrough
	case typeOutboxStream:
		label = "outbox"
	case typeInbox:
		fallthrough
	case typeInboxStream:
		label = "inbox"
	case typeLiked:
		fallthrough
	case typeLikedCollection:
		label = "liked"
	case typePerson:
		o := val.Interface().(ap.Person)
		for _, n := range o.Name {
			label = n.Value
			break
		}
	case typeLocalPerson:
		o := val.Interface().(Person)
		for _, n := range o.Name {
			label = n.Value
			break
		}
	}
	return label
}

func BuildObjectURL(b ap.LinkOrURI, el ap.Item) ap.URI {
	if el == nil {
		return ""
	}
	pURL := ap.URI(BaseURL)
	if b != nil && b.GetLink() != "" {
		pURL = b.GetLink()
	}

	return ap.URI(fmt.Sprintf("%s/%s", pURL, getObjectType(el)))
}

func HandleError(w http.ResponseWriter, r *http.Request, code int, errs ...error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)

	type error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	type eresp struct {
		Status int     `json:"status"`
		Errors []error `json:"errors"`
	}

	res := eresp{
		Status: code,
		Errors: []error{},
	}
	for _, err := range errs {
		e := error{
			Message: err.Error(),
		}
		log.WithFields(log.Fields{}).Error(err)
		res.Errors = append(res.Errors, e)
	}

	j, _ := json.Marshal(res)
	w.Write(j)
}
