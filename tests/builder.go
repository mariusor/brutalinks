package main

import (
	"time"

	pub "github.com/go-ap/activitypub"
)

type builder struct {
	col          map[string]*builder
	id           pub.ID
	typ          pub.ActivityVocabularyType
	name         pub.NaturalLanguageValues
	summary      pub.NaturalLanguageValues
	content      pub.NaturalLanguageValues
	attachment   *builder
	attributedTo *builder
	audience     []*builder
	context      *builder
	mediaType    pub.MimeType
	endTime      time.Time
	generator    *builder
	icon         *builder
	image        *builder
	inReplyTo    *builder
	location     *builder
	preview      *builder
	published    time.Time
	replies      *builder
	startTime    time.Time
	tag          []*builder
	updated      time.Time
	url          *builder
	duration     time.Duration
	source       pub.Source
	// Recipients
	to  []*builder
	bto []*builder
	cc  []*builder
	bcc []*builder
	// Object collections
	likes  *builder
	shares *builder
	// Actor collections
	inbox     *builder
	outbox    *builder
	following *builder
	followers *builder
	liked     *builder
	// Actor stuff
	endpoints pub.Endpoints
	streams   []*builder
	publicKey pub.PublicKey
	// Activity stuff
	object *builder
	actor  *builder
	target *builder
	// Collection stuff
	totalItems int
	items      []*builder
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

func (b *builder) Items(it ...*builder) *builder {
	if b.items == nil && len(it) > 0 {
		b.items = make([]*builder, 0)
	}
	b.items = append(b.items, it...)
	b.totalItems = len(it)
	return b
}

func (b *builder) copyToCollection(col *pub.OrderedCollection) error {
	col.TotalItems = uint(b.totalItems)
	for _, b := range b.items {
		col.OrderedItems = append(col.OrderedItems, b.Build())
	}
	col.First = b.first
	col.Last = b.last
	return pub.OnObject(col, b.copyToObject)
}

func (b *builder) copyToActivity(act *pub.Activity) error {
	act.Actor = b.actor.Build()
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
	for _, ba := range b.streams {
		act.Streams = append(act.Streams, ba.Build())
	}
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
	for _, ba := range b.audience {
		ob.Audience = append(ob.Audience, ba.Build())
	}
	ob.Context = b.context
	ob.MediaType = b.mediaType
	ob.Generator = b.generator
	ob.Icon = b.icon
	ob.Image = b.image
	ob.Preview = b.preview
	ob.Published = b.published
	ob.Updated = b.updated
	for _, bt := range b.to {
		ob.To = append(ob.To, bt.Build())
	}
	for _, bt := range b.cc {
		ob.CC = append(ob.CC, bt.Build())
	}
	for _, bt := range b.bto {
		ob.Bto = append(ob.Bto, bt.Build())
	}
	for _, bt := range b.bcc {
		ob.BCC = append(ob.BCC, bt.Build())
	}
	for _, bt := range b.tag {
		ob.Tag = append(ob.Tag, bt.Build())
	}
	ob.Shares = b.shares
	ob.Likes = b.likes
	ob.Source = b.source
	return nil
}

func (b *builder) Build() pub.Item {
	it, _ := pub.GetItemByType(b.typ)
	if contains(pub.CollectionTypes, b.typ) {
		pub.OnOrderedCollection(it, b.copyToCollection)
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
