package app

import (
	"encoding/base64"
	"encoding/pem"
	"fmt"
	ap "github.com/mariusor/littr.go/activitypub"
	"net/url"
	"path"
	"strings"

	goap "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
)

type Converter interface {
	FromActivityPub(ob goap.Item) error
}

func (h *Hash) FromActivityPub(it goap.Item) error {
	if it.GetLink() == goap.PublicNS {
		*h = AnonymousHash
		return nil
	}
	*h = GetHashFromAP(it.GetLink())
	return nil
}

func (a *Account) FromActivityPub(it goap.Item) error {
	if a == nil {
		return nil
	}
	if it == nil {
		return errors.Newf("nil item received")
	}
	if it.IsLink() {
		iri := it.GetLink()
		if iri == goap.PublicNS {
			*a = AnonymousAccount
		}
		a.Hash.FromActivityPub(iri)
		a.Metadata = &AccountMetadata{
			ID: iri.String(),
		}
		return nil
	}
	// FIXME(marius): This whole thing needs to be changed to OnXX functions.
	personFn := func(a *Account, fnAo func(a *Account, p goap.Object) error, fnAp func(a *Account, p goap.Person) error, fnLocal func(a *Account, p ap.Actor) error) error {
		if pp, ok := it.(ap.Actor); ok {
			return fnLocal(a, pp)
		}
		if pp, ok := it.(*ap.Actor); ok {
			return fnLocal(a, *pp)
		}
		if pp, ok := it.(goap.Person); ok {
			return fnAp(a, pp)
		}
		if pp, ok := it.(*goap.Person); ok {
			return fnAp(a, *pp)
		}
		if pp, ok := it.(ap.Object); ok {
			return fnAo(a, pp.Object)
		}
		if pp, ok := it.(*ap.Object); ok {
			return fnAo(a, pp.Object)
		}
		if pp, ok := it.(goap.Object); ok {
			return fnAo(a, pp)
		}
		if pp, ok := it.(*goap.Object); ok {
			return fnAo(a, *pp)
		}
		return nil
	}
	loadFromObject := func(a *Account, p goap.Object) error {
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
				ap.OnObject(p.Icon, func(o *ap.Object) error {
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

		if it.GetType() == goap.TombstoneType {
			a.Handle = Anonymous
			a.Flags = a.Flags & FlagsDeleted
		}
		if !p.Published.IsZero() {
			a.CreatedAt = p.Published
		}
		if !p.Updated.IsZero() {
			a.UpdatedAt = p.Updated
		}
		return nil
	}
	loadFromPerson := func(a *Account, p goap.Actor) error {
		goap.OnObject(&p, func(o *goap.Object) error {
			return loadFromObject(a, *o)
		})
		pName := p.PreferredUsername.First().Value
		if pName == "" {
			pName = p.Name.First().Value
		}
		a.Handle = pName
		if a.Metadata == nil {
			a.Metadata = &AccountMetadata{}
		}
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
		return nil
	}
	loadFromLocal := func(a *Account, p ap.Actor) error {
		if err := loadFromPerson(a, p.Actor); err != nil {
			return err
		}
		a.Score = p.Score
		if a.Metadata == nil {
			a.Metadata = &AccountMetadata{}
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
	switch it.GetType() {
	case goap.CreateType:
		fallthrough
	case goap.UpdateType:
		return goap.OnActivity(it, func(act *goap.Activity) error {
			return a.FromActivityPub(act.Actor)
		})
	case goap.ServiceType:
		fallthrough
	case goap.GroupType:
		fallthrough
	case goap.ApplicationType:
		fallthrough
	case goap.OrganizationType:
		fallthrough
	case goap.TombstoneType:
		fallthrough
	case goap.PersonType:
		personFn(a, loadFromObject, loadFromPerson, loadFromLocal)
	default:
		return errors.Newf("invalid actor type")
	}

	if a.HasMetadata() && a.Metadata.URL == a.Metadata.ID {
		a.Metadata.URL = ""
	}

	return nil
}

func ToArticle(it goap.Item) (*ap.Object, error) {
	switch i := it.(type) {
	case *ap.Object:
		return i, nil
	case ap.Object:
		return &i, nil
	default:
		ob, err := goap.ToObject(it)
		return &ap.Object{
			Object: *ob,
		}, err
	}
	return nil, errors.Newf("unable to convert Object")
}

type onArticleFn func(art *ap.Object) error

func OnArticle(it goap.Item, fn onArticleFn) error {
	if !goap.ObjectTypes.Contains(it.GetType()) {
		return errors.Newf(fmt.Sprintf("%T[%s] can't be converted to Object", it, it.GetType()))
	}
	act, err := ToArticle(it)
	if err != nil {
		return err
	}
	return fn(act)
}

func FromArticle(i *Item, a *ap.Object) error {
	title := a.Name.First().Value

	i.Hash.FromActivityPub(a)
	if len(title) > 0 {
		i.Title = title
	}
	i.MimeType = MimeTypeHTML
	if a.Type == goap.PageType {
		i.Data = string(a.URL.GetLink())
		i.MimeType = MimeTypeURL
	} else {
		if len(a.MediaType) > 0 {
			i.MimeType = MimeType(a.MediaType)
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
			if ic, ok := a.Icon.(*goap.Object); ok {
				i.Metadata.Icon.MimeType = string(ic.MediaType)
				i.Metadata.Icon.URI = ic.URL.GetLink().String()
			}
			if ic, ok := a.Icon.(goap.Object); ok {
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
		if repl, ok := a.InReplyTo.(goap.ItemCollection); ok {
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
		i.MimeType = MimeType(a.Source.MediaType)
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
	i.Flags |= FlagsPrivate
	for _, rec := range a.To {
		if rec == goap.PublicNS {
			i.Flags ^= FlagsPrivate
		}
		i.Metadata.To = rec.GetLink().String()
	}

	i.Score = a.Score
	return nil
}

func (i *Item) FromActivityPub(it goap.Item) error {
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

	switch it.GetType() {
	case goap.DeleteType:
		return goap.OnActivity(it, func(act *goap.Activity) error {
			err := i.FromActivityPub(act.Object)
			i.Delete()
			return err
		})
	case goap.CreateType:
		fallthrough
	case goap.UpdateType:
		fallthrough
	case goap.ActivityType:
		return goap.OnActivity(it, func(act *goap.Activity) error {
			// TODO(marius): this logic is probably broken if the activity is anything else except a Create
			good := goap.ActivityVocabularyTypes{goap.CreateType, goap.UpdateType}
			if !good.Contains(act.Type) {
				return errors.Newf("Invalid activity to load from %s", act.Type)
			}
			err := i.FromActivityPub(act.Object)
			i.SubmittedBy.FromActivityPub(act.Actor)
			if i.Metadata == nil {
				i.Metadata = &ItemMetadata{}
			}
			i.Metadata.AuthorURI = act.Actor.GetLink().String()
			return err
		})
	case goap.ArticleType:
		fallthrough
	case goap.NoteType:
		fallthrough
	case goap.DocumentType:
		fallthrough
	case goap.PageType:
		return OnArticle(it, func(a *ap.Object) error {
			return FromArticle(i, a)
		})
	case goap.TombstoneType:
		id := it.GetLink()
		i.Hash.FromActivityPub(id)
		if i.Metadata == nil {
			i.Metadata = &ItemMetadata{}
		}
		if len(id) > 0 {
			i.Metadata.ID = id.String()
		}
		goap.OnObject(it, func(o *goap.Object) error {
			if o.Context != nil {
				op := Item{}
				op.FromActivityPub(o.Context)
				i.OP = &op
			}
			if o.InReplyTo != nil {
				if repl, ok := o.InReplyTo.(goap.ItemCollection); ok {
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

func (v *Vote) FromActivityPub(it goap.Item) error {
	if it == nil {
		return errors.Newf("nil item received")
	}
	if it.IsLink() {
		return errors.Newf("unable to load from IRI")
	}
	switch it.GetType() {
	case goap.UndoType:
		fallthrough
	case goap.LikeType:
		fallthrough
	case goap.DislikeType:
		fromAct := func(act goap.Activity, v *Vote) {
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

			if act.Type == goap.LikeType {
				v.Weight = 1
			}
			if act.Type == goap.DislikeType {
				v.Weight = -1
			}
			if act.Type == goap.UndoType {
				v.Weight = 0
				v.Metadata.OriginalIRI = act.Object.GetLink().String()
			}
		}
		goap.OnActivity(it, func(act *goap.Activity) error {
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

func GetHashFromAP(obj goap.Item) Hash {
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

func (i *TagCollection) FromActivityPub(it goap.ItemCollection) error {
	if it == nil || len(it) == 0 {
		return errors.Newf("empty collection")
	}
	for _, t := range it {
		if m, ok := t.(*goap.Mention); ok {
			u := string(t.GetID())
			// we have a link
			lt := Tag{
				URL:  u,
				Type: TagMention,
				Name: m.Name.First().Value,
			}
			*i = append(*i, lt)
		}
		if ob, ok := t.(*goap.Object); ok {
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
