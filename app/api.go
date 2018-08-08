package app

import (
	"net/http"
	"fmt"
	"io/ioutil"
	ap "github.com/mariusor/activitypub.go/activitypub"
	j "github.com/mariusor/activitypub.go/jsonld"
	"os"
	log "github.com/sirupsen/logrus"
	"github.com/mariusor/littr.go/models"
	"bytes"
	"strings"
	"time"
	"github.com/buger/jsonparser"
)

type (
	ObjectID ap.ObjectID
	ActivityVocabularyType ap.ActivityVocabularyType
	NaturalLanguageValue ap.NaturalLanguageValue
	ObjectOrLink ap.ObjectOrLink
	LinkOrURI ap.LinkOrURI
	ImageOrLink ap.ImageOrLink
	MimeType ap.MimeType
	ObjectsArr ap.ObjectsArr
	CollectionInterface ap.CollectionInterface
	Endpoints ap.Endpoints
)

// Person it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/actors.go#Actor
// We need it here in order to be able to add to it our Score property
type Person struct {
	ID ObjectID `jsonld:"id,omitempty"`
	Type ActivityVocabularyType `jsonld:"type,omitempty"`
	Name NaturalLanguageValue `jsonld:"name,omitempty,collapsible"`
	Attachment ObjectOrLink `jsonld:"attachment,omitempty"`
	AttributedTo ObjectOrLink `jsonld:"attributedTo,omitempty"`
	Audience ObjectOrLink `jsonld:"audience,omitempty"`
	Content NaturalLanguageValue `jsonld:"content,omitempty,collapsible"`
	Context ObjectOrLink `jsonld:"_"`
	EndTime time.Time `jsonld:"endTime,omitempty"`
	Generator ObjectOrLink `jsonld:"generator,omitempty"`
	Icon ImageOrLink `jsonld:"icon,omitempty"`
	Image ImageOrLink `jsonld:"image,omitempty"`
	InReplyTo ObjectOrLink `jsonld:"inReplyTo,omitempty"`
	Location ObjectOrLink `jsonld:"location,omitempty"`
	Preview ObjectOrLink `jsonld:"preview,omitempty"`
	Published time.Time `jsonld:"published,omitempty"`
	Replies ObjectOrLink `jsonld:"replies,omitempty"`
	StartTime time.Time `jsonld:"startTime,omitempty"`
	Summary NaturalLanguageValue `jsonld:"summary,omitempty,collapsible"`
	Tag ObjectOrLink `jsonld:"tag,omitempty"`
	Updated time.Time `jsonld:"updated,omitempty"`
	URL LinkOrURI `jsonld:"url,omitempty"`
	To ObjectsArr `jsonld:"to,omitempty"`
	Bto ObjectsArr `jsonld:"bto,omitempty"`
	CC ObjectsArr `jsonld:"cc,omitempty"`
	BCC ObjectsArr `jsonld:"bcc,omitempty"`
	Duration time.Duration `jsonld:"duration,omitempty"`
	Inbox CollectionInterface `jsonld:"inbox,omitempty"`
	Outbox CollectionInterface `jsonld:"outbox,omitempty"`
	Following CollectionInterface `jsonld:"following,omitempty"`
	Followers CollectionInterface `jsonld:"followers,omitempty"`
	Liked CollectionInterface `jsonld:"liked,omitempty"`
	PreferredUsername NaturalLanguageValue `jsonld:"preferredUsername,omitempty,collapsible"`
	Endpoints Endpoints `jsonld:"endpoints,omitempty"`
	Streams []CollectionInterface `jsonld:"streams,omitempty"`
	// Score is our own custom property for which we needed to extend the existing AP one
	Score int64	`jsonld:"score"`
}

// Article it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/objects.go#Object
// We need it here in order to be able to add to it our Score property
type Article struct {
	ID ObjectID `jsonld:"id,omitempty"`
	Type ActivityVocabularyType `jsonld:"type,omitempty"`
	Name NaturalLanguageValue `jsonld:"name,omitempty,collapsible"`
	Attachment ObjectOrLink `jsonld:"attachment,omitempty"`
	AttributedTo ObjectOrLink `jsonld:"attributedTo,omitempty"`
	Audience ObjectOrLink `jsonld:"audience,omitempty"`
	Content NaturalLanguageValue `jsonld:"content,omitempty,collapsible"`
	Context ObjectOrLink `jsonld:"context,omitempty"`
	MediaType MimeType `jsonld:"mediaType,omitempty"`
	EndTime time.Time `jsonld:"endTime,omitempty"`
	Generator ObjectOrLink `jsonld:"generator,omitempty"`
	Icon ImageOrLink `jsonld:"icon,omitempty"`
	Image ImageOrLink `jsonld:"image,omitempty"`
	InReplyTo ObjectOrLink `jsonld:"inReplyTo,omitempty"`
	Location ObjectOrLink `jsonld:"location,omitempty"`
	Preview ObjectOrLink `jsonld:"preview,omitempty"`
	Published time.Time `jsonld:"published,omitempty"`
	Replies ObjectOrLink `jsonld:"replies,omitempty"`
	StartTime time.Time `jsonld:"startTime,omitempty"`
	Summary NaturalLanguageValue `jsonld:"summary,omitempty,collapsible"`
	Tag ObjectOrLink `jsonld:"tag,omitempty"`
	Updated time.Time `jsonld:"updated,omitempty"`
	URL LinkOrURI `jsonld:"url,omitempty"`
	To ObjectsArr `jsonld:"to,omitempty"`
	Bto ObjectsArr `jsonld:"bto,omitempty"`
	CC ObjectsArr `jsonld:"cc,omitempty"`
	BCC ObjectsArr `jsonld:"bcc,omitempty"`
	Duration time.Duration `jsonld:"duration,omitempty"`
	// Score is our own custom property for which we needed to extend the existing AP one
	Score int64	`jsonld:"score"`
}

func (a Article)GetID() *ap.ObjectID{
	id := ap.ObjectID(a.ID)
	return &id
}
func (a Article)GetType() ap.ActivityVocabularyType {
	return ap.ActivityVocabularyType(a.Type)
}
func (a Article)IsLink() bool {
	return false
}
func (a Article)IsObject() bool {
	return true
}

// OrderedCollection it should be identical to:
//    github.com/mariusor/activitypub.go/activitypub/collections.go#OrderedCollection
// We need it here in order to be able to implement our own UnmarshalJSON() method
type OrderedCollection struct {
	ID ObjectID `jsonld:"id,omitempty"`
	Type ActivityVocabularyType `jsonld:"type,omitempty"`
	Name NaturalLanguageValue `jsonld:"name,omitempty,collapsible"`
	Attachment ObjectOrLink `jsonld:"attachment,omitempty"`
	AttributedTo ObjectOrLink `jsonld:"attributedTo,omitempty"`
	Audience ObjectOrLink `jsonld:"audience,omitempty"`
	Content NaturalLanguageValue `jsonld:"content,omitempty,collapsible"`
	Context ObjectOrLink `jsonld:"_"`
	EndTime time.Time `jsonld:"endTime,omitempty"`
	Generator ObjectOrLink `jsonld:"generator,omitempty"`
	InReplyTo ObjectOrLink `jsonld:"inReplyTo,omitempty"`
	Location ObjectOrLink `jsonld:"location,omitempty"`
	Preview ObjectOrLink `jsonld:"preview,omitempty"`
	Published time.Time `jsonld:"published,omitempty"`
	Replies ObjectOrLink `jsonld:"replies,omitempty"`
	Summary NaturalLanguageValue `jsonld:"summary,omitempty,collapsible"`
	Tag ObjectOrLink `jsonld:"tag,omitempty"`
	Updated time.Time `jsonld:"updated,omitempty"`
	URL LinkOrURI `jsonld:"url,omitempty"`
	Duration time.Duration `jsonld:"duration,omitempty"`
	TotalItems uint `jsonld:"totalItems,omitempty"`
	OrderedItems []Article `jsonld:"orderedItems,omitempty"`
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
	for i, it := range col.OrderedItems {
		score, _ := jsonparser.GetInt(data, "orderedItems", fmt.Sprintf("[%d]", i), "score")
		el, success := it.(*ap.Object)
		if !success {
			continue
		}
		o.OrderedItems[i] = Article{
			ID: ObjectID(*it.GetID()),
			Type: ActivityVocabularyType(it.GetType()),
			Name: NaturalLanguageValue(el.Name),
			Content: NaturalLanguageValue(el.Content),
			Context: el.Context,
			Generator: el.Generator,
			AttributedTo: el.AttributedTo,
			Published: el.Published,
			MediaType: MimeType(el.MediaType),
			Score: score,
		}
	}
	return nil
}

func getHash(i *ap.ObjectID) []byte {
	if i == nil {
		return nil
	}
	s := bytes.Split([]byte(*i), []byte("/"))
	return s[len(s)-1]
}
func getAccountHandle(o ap.ObjectOrLink) string {
	if o == nil {
		return ""
	}
	i := o.(ap.IRI)
	s := strings.Split(string(i), "/")
	return s[len(s)-1]
}

func LoadOPItems() ([]models.Content, error) {
	apiBaseUrl := os.Getenv("LISTEN")

	var err error
	resp, err := http.Get(fmt.Sprintf("http://localhost%s/api/outbox", apiBaseUrl))
	if err != nil {
		log.Error(err)
		return nil, err
	}
	col := OrderedCollection{}
	if resp != nil {
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Error(err)
			return nil, err
		}
		err = j.Unmarshal(body, &col)
		if err != nil {
			log.Error(err)
			return nil, err
		}
	}

	items := make([]models.Content, col.TotalItems)
	for k, it := range col.OrderedItems {
		c := models.Content{
			Id: int64(k),
			Title: []byte(ap.NaturalLanguageValue(it.Name).First()),
			MimeType: string(it.MediaType),
			Data: []byte(ap.NaturalLanguageValue(it.Content).First()),
			Score: it.Score,
			SubmittedAt: it.Published,
			SubmittedByAccount: &models.Account{
				Handle: getAccountHandle(it.AttributedTo),
			},
		}
		c.Key.FromBytes(getHash(it.GetID()))

		items[k] = c
	}

	return items, nil
}
