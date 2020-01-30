package app

import (
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/go-ap/handlers"
	"net/url"
	"path"
	"strings"

	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
)

type Converter interface {
	FromActivityPub(ob pub.Item) error
}

func (h *Hash) FromActivityPub(it pub.Item) error {
	if it.GetLink() == pub.PublicNS {
		*h = AnonymousHash
		return nil
	}
	*h = GetHashFromAP(it.GetLink())
	return nil
}

func FromActor(a *Account, p *pub.Actor) error {
	name := p.Name.First().Value
	a.Hash.FromActivityPub(p)
	if len(name) > 0 {
		a.Handle = name
	}
	a.Flags = FlagsNone
	if a.Metadata == nil {
		a.Metadata = &AccountMetadata{}
	}
	if len(p.ID) > 0 {
		iri := p.GetLink()
		a.Metadata.ID = iri.String()
		if p.URL != nil {
			a.Metadata.URL = p.URL.GetLink().String()
		}
		if !HostIsLocal(a.Metadata.ID) {
			a.Metadata.Name = name
		}
	}
	if p.Icon != nil {
		if p.Icon.IsObject() {
			pub.OnObject(p.Icon, func(o *pub.Object) error {
				a.Metadata.Icon.MimeType = string(o.MediaType)
				a.Metadata.Icon.URI = o.URL.GetLink().String()
				return nil
			})
		}
	}
	if a.Email == "" {
		// @TODO(marius): this returns false positives when API_URL is set and different than
		host := host(a.Metadata.URL)
		a.Email = fmt.Sprintf("%s@%s", a.Handle, host)
	}
	if p.GetType() == pub.TombstoneType {
		a.Handle = Anonymous
		a.Flags = a.Flags & FlagsDeleted
	}
	if !p.Published.IsZero() {
		a.CreatedAt = p.Published
	}
	if !p.Updated.IsZero() {
		a.UpdatedAt = p.Updated
	}
	pName := p.PreferredUsername.First().Value
	if pName == "" {
		pName = p.Name.First().Value
	}
	a.Handle = pName
	if len(a.Metadata.URL) > 0 {
		host := host(a.Metadata.URL)
		a.Email = fmt.Sprintf("%s@%s", a.Handle, host)
	}
	if p.Inbox != nil {
		a.Metadata.InboxIRI = p.Inbox.GetLink().String()
	}
	if p.Outbox != nil {
		a.Metadata.OutboxIRI = p.Outbox.GetLink().String()
	}
	if p.Followers != nil {
		a.Metadata.FollowersIRI = p.Followers.GetLink().String()
	}
	if p.Following != nil {
		a.Metadata.FollowingIRI = p.Following.GetLink().String()
	}
	if p.Liked != nil {
		a.Metadata.LikedIRI = p.Liked.GetLink().String()
	}
	if block, _ := pem.Decode([]byte(p.PublicKey.PublicKeyPem)); block != nil {
		pub := make([]byte, base64.StdEncoding.EncodedLen(len(block.Bytes)))
		base64.StdEncoding.Encode(pub, block.Bytes)
		a.Metadata.Key = &SSHKey{
			Public: pub,
		}
	}
	return nil
}

func (a *Account) FromActivityPub(it pub.Item) error {
	if a == nil {
		return nil
	}
	a.pub, _ = it.(*pub.Person)
	if it == nil {
		return errors.Newf("nil item received")
	}
	if it.IsLink() {
		iri := it.GetLink()
		if iri == pub.PublicNS {
			*a = AnonymousAccount
		}
		a.Hash.FromActivityPub(iri)
		a.Metadata = &AccountMetadata{
			ID: iri.String(),
		}
		return nil
	}
	switch it.GetType() {
	case pub.CreateType:
		fallthrough
	case pub.UpdateType:
		return pub.OnActivity(it, func(act *pub.Activity) error {
			return a.FromActivityPub(act.Actor)
		})
	case pub.ServiceType:
		fallthrough
	case pub.GroupType:
		fallthrough
	case pub.ApplicationType:
		fallthrough
	case pub.OrganizationType:
		fallthrough
	case pub.TombstoneType:
		fallthrough
	case pub.PersonType:
		return pub.OnActor(it, func(p *pub.Actor) error {
			return FromActor(a, p)
		})
	default:
		return errors.Newf("invalid actor type")
	}

	if a.HasMetadata() && a.Metadata.URL == a.Metadata.ID {
		a.Metadata.URL = ""
	}

	return nil
}

func FromArticle(i *Item, a *pub.Object) error {
	title := a.Name.First().Value

	i.Hash.FromActivityPub(a)
	if len(title) > 0 {
		i.Title = title
	}
	i.MimeType = MimeTypeHTML
	if a.Type == pub.PageType {
		i.Data = string(a.URL.GetLink())
		i.MimeType = MimeTypeURL
	} else {
		if len(a.MediaType) > 0 {
			i.MimeType = string(a.MediaType)
		}
		i.Data = a.Content.First().Value
	}
	i.SubmittedAt = a.Published
	i.UpdatedAt = a.Updated
	if i.Metadata == nil {
		i.Metadata = &ItemMetadata{}
	}

	if a.AttributedTo != nil {
		auth := Account{Metadata: &AccountMetadata{}}
		auth.FromActivityPub(a.AttributedTo)
		i.SubmittedBy = &auth
		i.Metadata.AuthorURI = a.AttributedTo.GetLink().String()
	}
	if len(a.ID) > 0 {
		iri := a.GetLink()
		i.Metadata.ID = iri.String()
		if a.URL != nil {
			i.Metadata.URL = a.URL.GetLink().String()
		}
	}
	if a.Icon != nil {
		if a.Icon.IsObject() {
			if ic, ok := a.Icon.(*pub.Object); ok {
				i.Metadata.Icon.MimeType = string(ic.MediaType)
				i.Metadata.Icon.URI = ic.URL.GetLink().String()
			}
			if ic, ok := a.Icon.(pub.Object); ok {
				i.Metadata.Icon.MimeType = string(ic.MediaType)
				i.Metadata.Icon.URI = ic.URL.GetLink().String()
			}
		}
	}
	if a.Context != nil {
		op := Item{}
		op.FromActivityPub(a.Context)
		i.OP = &op
	}
	if a.InReplyTo != nil {
		if repl, ok := a.InReplyTo.(pub.ItemCollection); ok {
			if len(repl) >= 1 {
				first := repl.First()
				if first != nil {
					par := Item{}
					par.FromActivityPub(first)
					i.Parent = &par
					if i.OP == nil {
						i.OP = &par
					}
				}
			}
		} else {
			par := Item{}
			par.FromActivityPub(a.InReplyTo)
			i.Parent = &par
			if i.OP == nil {
				i.OP = &par
			}
		}
	}
	// TODO(marius): here we seem to have a bug, when Source.Content is nil when it shouldn't
	//    to repro, I used some copy/pasted comments from console javascript
	if len(a.Source.Content) > 0 && len(a.Source.MediaType) > 0 {
		i.Data = a.Source.Content.First().Value
		i.MimeType = string(a.Source.MediaType)
	}
	if a.Tag != nil && len(a.Tag) > 0 {
		i.Metadata.Tags = make(TagCollection, 0)
		i.Metadata.Mentions = make(TagCollection, 0)

		tags := TagCollection{}
		tags.FromActivityPub(a.Tag)
		for _, t := range tags {
			if t.Type == TagTag {
				i.Metadata.Tags = append(i.Metadata.Tags, t)
			}
			if t.Type == TagMention {
				i.Metadata.Mentions = append(i.Metadata.Mentions, t)
			}
		}
	}
	loadRecipients(i, a)

	return nil
}

func loadRecipientsFrom(recipients pub.ItemCollection) ([]*Account, bool) {
	result := make([]*Account, 0)
	isPublic := false
	for _, rec := range recipients {
		recURL, err := rec.GetLink().URL()
		if err != nil {
			continue
		}
		if rec == pub.PublicNS {
			isPublic = true
			continue
		}
		maybeCol := path.Base(recURL.Path)
		if handlers.ValidCollection(maybeCol) {
			if handlers.CollectionType(maybeCol) != handlers.Followers && handlers.CollectionType(maybeCol) != handlers.Following {
				// we don't know how to handle collections that don't contain accounts
				continue
			}
			acc := Account{
				Metadata: &AccountMetadata{
					ID: rec.GetLink().String(),
				},
			}
			result = append(result, &acc)
		} else {
			acc := Account{}
			acc.FromActivityPub(rec)
			if acc.IsValid() {
				result = append(result, &acc)
			}
		}
	}
	return result, isPublic
}

func loadRecipients (i *Item, it pub.Item) error {
	i.MakePrivate()
	return pub.OnObject(it, func(o *pub.Object) error {
		isPublic := false
		i.Metadata.To, isPublic = loadRecipientsFrom(o.To)
		if isPublic {
			i.MakePublic()
		}
		i.Metadata.CC, isPublic = loadRecipientsFrom(o.CC)
		if isPublic {
			i.MakePublic()
		}
		return nil
	})
}

func (i *Item) FromActivityPub(it pub.Item) error {
	// TODO(marius): see that we seem to have this functionality duplicated in the FromArticle() function
	if it == nil {
		return errors.Newf("nil item received")
	}
	if it.IsLink() {
		i.Hash.FromActivityPub(it.GetLink())
		i.Metadata = &ItemMetadata{
			ID: it.GetLink().String(),
		}
		return nil
	}
	i.pub, _ = it.(*pub.Object)
	switch it.GetType() {
	case pub.DeleteType:
		return pub.OnActivity(it, func(act *pub.Activity) error {
			err := i.FromActivityPub(act.Object)
			i.Delete()
			return err
		})
	case pub.CreateType:
		fallthrough
	case pub.UpdateType:
		fallthrough
	case pub.ActivityType:
		return pub.OnActivity(it, func(act *pub.Activity) error {
			// TODO(marius): this logic is probably broken if the activity is anything else except a Create
			good := pub.ActivityVocabularyTypes{pub.CreateType, pub.UpdateType}
			if !good.Contains(act.Type) {
				return errors.Newf("Invalid activity to load from %s", act.Type)
			}
			err := i.FromActivityPub(act.Object)
			i.SubmittedBy.FromActivityPub(act.Actor)
			if i.Metadata == nil {
				i.Metadata = &ItemMetadata{}
			}
			i.Metadata.AuthorURI = act.Actor.GetLink().String()
			loadRecipients(i, act)
			return err
		})
	case pub.ArticleType:
		fallthrough
	case pub.NoteType:
		fallthrough
	case pub.DocumentType:
		fallthrough
	case pub.PageType:
		return pub.OnObject(it, func(a *pub.Object) error {
			return FromArticle(i, a)
		})
	case pub.TombstoneType:
		id := it.GetLink()
		i.Hash.FromActivityPub(id)
		if i.Metadata == nil {
			i.Metadata = &ItemMetadata{}
		}
		if len(id) > 0 {
			i.Metadata.ID = id.String()
		}
		pub.OnObject(it, func(o *pub.Object) error {
			if o.Context != nil {
				op := Item{}
				op.FromActivityPub(o.Context)
				i.OP = &op
			}
			if o.InReplyTo != nil {
				if repl, ok := o.InReplyTo.(pub.ItemCollection); ok {
					first := repl.First()
					if first != nil {
						par := Item{}
						par.FromActivityPub(first)
						i.Parent = &par
						if i.OP == nil {
							i.OP = &par
						}
					}
				} else {
					par := Item{}
					par.FromActivityPub(o.InReplyTo)
					i.Parent = &par
					if i.OP == nil {
						i.OP = &par
					}
				}
			}
			i.SubmittedAt = o.Published
			i.UpdatedAt = o.Updated
			return nil
		})

		i.Flags = FlagsDeleted
		i.SubmittedBy = &AnonymousAccount
	default:
		return errors.Newf("invalid object type %q", it.GetType())
	}

	return nil
}

func (v *Vote) FromActivityPub(it pub.Item) error {
	if it == nil {
		return errors.Newf("nil item received")
	}
	v.pub, _ = it.(*pub.Activity)
	if it.IsLink() {
		return errors.Newf("unable to load from IRI")
	}
	switch it.GetType() {
	case pub.UndoType:
		fallthrough
	case pub.LikeType:
		fallthrough
	case pub.DislikeType:
		fromAct := func(act pub.Activity, v *Vote) {
			on := Item{}
			on.FromActivityPub(act.Object)
			v.Item = &on

			er := Account{Metadata: &AccountMetadata{}}
			er.FromActivityPub(act.Actor)
			v.SubmittedBy = &er

			v.SubmittedAt = act.Published
			v.UpdatedAt = act.Updated
			v.Metadata = &VoteMetadata{
				IRI: act.GetLink().String(),
			}

			if act.Type == pub.LikeType {
				v.Weight = 1
			}
			if act.Type == pub.DislikeType {
				v.Weight = -1
			}
			if act.Type == pub.UndoType {
				v.Weight = 0
				v.Metadata.OriginalIRI = act.Object.GetLink().String()
			}
		}
		pub.OnActivity(it, func(act *pub.Activity) error {
			fromAct(*act, v)
			return nil
		})
	}

	return nil
}

func HostIsLocal(s string) bool {
	return strings.Contains(host(s), Instance.HostName) || strings.Contains(host(s), host(Instance.APIURL))
}

func host(u string) string {
	if pu, err := url.ParseRequestURI(u); err == nil {
		return pu.Host
	}
	return ""
}

func GetHashFromAP(obj pub.Item) Hash {
	iri := obj.GetLink()
	s := strings.Split(iri.String(), "/")
	var hash string
	if s[len(s)-1] == "object" {
		hash = s[len(s)-2]
	} else {
		hash = s[len(s)-1]
	}
	h := path.Base(hash)
	if h == "." {
		h = ""
	}
	return Hash(h)
}

func (i *TagCollection) FromActivityPub(it pub.ItemCollection) error {
	if it == nil || len(it) == 0 {
		return errors.Newf("empty collection")
	}
	for _, t := range it {
		if m, ok := t.(*pub.Mention); ok {
			u := string(t.GetID())
			// we have a link
			lt := Tag{
				URL:  u,
				Type: TagMention,
				Name: m.Name.First().Value,
			}
			*i = append(*i, lt)
		}
		if ob, ok := t.(*pub.Object); ok {
			u := string(t.GetID())
			// we have a link
			lt := Tag{
				URL:  u,
				Type: TagTag,
				Name: ob.Name.First().Value,
			}
			*i = append(*i, lt)
		}
	}
	return nil
}
