package main

import (
	"time"

	pub "github.com/go-ap/activitypub"
)

var root = &builder{}

type builder struct {
	col          map[string]*builder
	id           pub.ID
	typ          pub.ActivityVocabularyType
	name         pub.NaturalLanguageValues
	summary      pub.NaturalLanguageValues
	content      pub.NaturalLanguageValues
	attachment   pub.ItemCollection
	attributedTo pub.ItemCollection
	audience     pub.ItemCollection
	context      pub.ItemCollection
	mediaType    pub.MimeType
	endTime      time.Time
	generator    pub.ItemCollection
	icon         pub.ItemCollection
	image        pub.ItemCollection
	inReplyTo    pub.ItemCollection
	location     pub.ItemCollection
	preview      pub.ItemCollection
	published    time.Time
	replies      pub.ItemCollection
	startTime    time.Time
	tag          pub.ItemCollection
	updated      time.Time
	url          pub.ItemCollection
	duration     time.Duration
	source       pub.Source
	// Recipients
	to  pub.ItemCollection
	bto pub.ItemCollection
	cc  pub.ItemCollection
	bcc pub.ItemCollection
	// Object collections
	likes  pub.ItemCollection
	shares pub.ItemCollection
	// Actor collections
	inbox     pub.OrderedCollection
	outbox    pub.OrderedCollection
	following pub.OrderedCollection
	followers pub.OrderedCollection
	liked     pub.OrderedCollection
	// Actor stuff
	endpoints pub.Endpoints
	streams   pub.ItemCollection
	publicKey pub.PublicKey
	// Activity stuff
	object pub.ItemCollection
	actor  pub.ItemCollection
	target pub.ItemCollection
	// Collection stuff
	totalItems int
	items      pub.ItemCollection
	first      pub.IRI
	last       pub.IRI
	next       pub.IRI
	prev       pub.IRI
}

var _ pub.Item = &builder{}

func (b *builder) GetID() pub.ID {
	return b.id
}

func (b *builder) GetLink() pub.IRI {
	return b.id
}

func (b *builder) IsCollection() bool {
	return b.Build().IsCollection()
}

func (b *builder) GetType() pub.ActivityVocabularyType {
	return b.typ
}

func (b *builder) IsObject() bool {
	return b.Build().IsObject()
}

func (b *builder) IsLink() bool {
	return b.Build().IsLink()
}

func ap(id pub.ID) *builder {
	b := new(builder)
	return b.ID(id)
}

func (b *builder) Type(t pub.ActivityVocabularyType) *builder {
	b.typ = t
	return b
}

func appendToNaturalLanguageValues(nn *pub.NaturalLanguageValues, names ...pub.LangRefValue) {
	if nn == nil || len(*nn) == 0 {
		*nn = make(pub.NaturalLanguageValues, 0)
	}
	for _, name := range names {
		*nn = append(*nn, name)
	}
}

func (b *builder) Name(values ...pub.LangRefValue) *builder {
	appendToNaturalLanguageValues(&b.name, values...)
	return b
}

func (b *builder) Summary(values ...pub.LangRefValue) *builder {
	appendToNaturalLanguageValues(&b.summary, values...)
	return b
}

func (b *builder) Content(values ...pub.LangRefValue) *builder {
	appendToNaturalLanguageValues(&b.content, values...)
	return b
}

func (b *builder) ID(id pub.ID) *builder {
	b.id = id
	return b
}

//func (b *builder) collection(name string, cb *builder) *builder {
//	b.col[name] = cb
//	return b
//}

func (b *builder) Items(it ...pub.Item) *builder {
	if b.items == nil {
		b.items = make(pub.ItemCollection, 0)
	}
	b.items = append(b.items, it...)
	return b
}

func (b *builder) copyToCollection(col *pub.OrderedCollectionPage) error {
	col.TotalItems = uint(b.totalItems)
	col.OrderedItems = b.items
	col.First = b.first
	col.Last = b.last
	col.Next = b.next
	col.Prev = b.prev
	return pub.OnObject(col, b.copyToObject)
}

func (b *builder) copyToActivity(act *pub.Activity) error {
	act.Actor = b.actor
	act.Object = b.object
	act.Target = b.target
	return pub.OnObject(act, b.copyToObject)
}

func (b *builder) copyToActor(act *pub.Actor) error {
	act.PreferredUsername = b.name
	act.Endpoints = &b.endpoints
	act.Inbox = b.inbox
	act.Outbox = b.outbox
	act.Following = b.following
	act.Followers = b.followers
	act.Liked = b.liked
	act.Streams = b.streams
	act.PublicKey = b.publicKey
	return pub.OnObject(act, b.copyToObject)
}

func (b *builder) copyToObject(ob *pub.Object) error {
	ob.ID = b.id
	ob.URL = b.url
	ob.Type = b.typ
	ob.Name = b.name
	ob.Content = b.content
	ob.Duration = b.duration
	ob.StartTime = b.startTime
	ob.EndTime = b.endTime
	ob.Attachment = b.attachment
	ob.AttributedTo = b.attributedTo
	ob.InReplyTo = b.inReplyTo
	ob.Location = b.location
	ob.Audience = b.audience
	ob.Context = b.context
	ob.MediaType = b.mediaType
	ob.Generator = b.generator
	ob.Icon = b.icon
	ob.Image = b.image
	ob.Preview = b.preview
	ob.Published = b.published
	ob.Updated = b.updated
	ob.To = b.to
	ob.CC = b.cc
	ob.Bto = b.bto
	ob.BCC = b.bcc
	ob.Tag = b.tag
	ob.Shares = b.shares
	ob.Likes = b.likes
	ob.Source = b.source
	return nil
}

func (b *builder) Build() pub.Item {
	it, _ := pub.GetItemByType(b.typ)
	if contains(pub.CollectionTypes, b.typ) {
		pub.OnOrderedCollectionPage(it, b.copyToCollection)
	}
	if contains(pub.ActivityTypes, b.typ) {
		pub.OnActivity(it, b.copyToActivity)
	}
	if contains(pub.ActorTypes, b.typ) {
		pub.OnActor(it, b.copyToActor)
	}
	if contains(pub.ObjectTypes, b.typ) {
		pub.OnObject(it, b.copyToObject)
	}
	return it
}
