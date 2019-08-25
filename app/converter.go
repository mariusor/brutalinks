package app

import (
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/url"
	"path"
	"strings"

	ap "github.com/mariusor/littr.go/app/activitypub"

	"github.com/buger/jsonparser"
	"github.com/go-ap/errors"

	goap "github.com/go-ap/activitypub"
	as "github.com/go-ap/activitystreams"
)

type Converter interface {
	FromActivityPub(ob as.Item) error
}

func (h *Hash) FromActivityPub(it as.Item) error {
	*h = GetHashFromAP(it.GetLink())
	return nil
}

func (a *Account) FromActivityPub(it as.Item) error {
	if a == nil {
		return nil
	}
	if it == nil {
		return errors.Newf("nil item received")
	}
	if it.IsLink() {
		iri := it.GetLink()
		a.Hash.FromActivityPub(iri)
		a.Metadata = &AccountMetadata{
			ID: iri.String(),
		}
		return nil
	}
	personFn := func(a *Account, fnAs func(a *Account, p as.Object) error, fnAp func(a *Account, p goap.Person) error, fnLocal func(a *Account, p ap.Person) error) error {
		if pp, ok := it.(ap.Person); ok {
			return fnLocal(a, pp)
		}
		if pp, ok := it.(*ap.Person); ok {
			return fnLocal(a, *pp)
		}
		if pp, ok := it.(goap.Person); ok {
			return fnAp(a, pp)
		}
		if pp, ok := it.(*goap.Person); ok {
			return fnAp(a, *pp)
		}
		if pp, ok := it.(as.Object); ok {
			return fnAs(a, pp)
		}
		if pp, ok := it.(*as.Object); ok {
			return fnAs(a, *pp)
		}
		return nil
	}
	loadFromObject := func(a *Account, p as.Object) error {
		name := jsonUnescape(p.Name.First().Value)
		a.Hash.FromActivityPub(p)
		a.Handle = name
		a.Flags = FlagsNone
		if a.Metadata == nil {
			a.Metadata = &AccountMetadata{}
		}
		if len(p.ID) > 0 {
			iri := p.GetLink()
			a.Metadata.ID = iri.String()
			a.Metadata.URL = p.URL.GetLink().String()
			if !HostIsLocal(a.Metadata.ID) {
				a.Metadata.Name = name
			}
		}
		if p.Icon != nil {
			if p.Icon.IsObject() {
				if ic, ok := p.Icon.(*as.Object); ok {
					a.Metadata.Icon.MimeType = string(ic.MediaType)
					a.Metadata.Icon.URI = ic.URL.GetLink().String()
				}
				if ic, ok := p.Icon.(as.Object); ok {
					a.Metadata.Icon.MimeType = string(ic.MediaType)
					a.Metadata.Icon.URI = ic.URL.GetLink().String()
				}
			}
		}
		if a.IsFederated() {
			// @TODO(marius): this returns false positives when API_URL is set and different than
			host := host(a.Metadata.URL)
			a.Email = fmt.Sprintf("%s@%s", a.Handle, host)
		}

		if !p.Published.IsZero() {
			a.CreatedAt = p.Published
		}
		if !p.Updated.IsZero() {
			a.UpdatedAt = p.Updated
		}
		return nil
	}
	loadFromPerson := func(a *Account, p goap.Person) error {
		if err := loadFromObject(a, p.Parent); err != nil {
			return err
		}
		pName := jsonUnescape(p.PreferredUsername.First().Value)
		if pName == "" {
			pName = jsonUnescape(p.Name.First().Value)
		}
		a.Handle = pName
		if a.IsFederated() {
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
		}
		return nil
	}
	loadFromLocal := func(a *Account, p ap.Person) error {
		if err := loadFromPerson(a, p.Person); err != nil {
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
	case as.CreateType:
		fallthrough
	case as.UpdateType:
		if act, ok := it.(*ap.Activity); ok {
			return a.FromActivityPub(act.Actor)
		}
		if act, ok := it.(ap.Activity); ok {
			return a.FromActivityPub(act.Actor)
		}
	case as.ServiceType:
		fallthrough
	case as.GroupType:
		fallthrough
	case as.ApplicationType:
		fallthrough
	case as.OrganizationType:
		fallthrough
	case as.PersonType:
		personFn(a, loadFromObject, loadFromPerson, loadFromLocal)
	default:
		return errors.Newf("invalid actor type")
	}

	return nil
}

func (i *Item) FromActivityPub(it as.Item) error {
	if it == nil {
		return errors.Newf("nil item received")
	}
	if it.IsLink() {
		i.Hash.FromActivityPub(it.GetLink())
		return nil
	}
	if i.SubmittedBy == nil {
		i.SubmittedBy = &Account{}
	}

	articleFn := func(a *Item, fnAs func(i *Item, a as.Object) error, fnAp func(i *Item, a ap.Article) error) error {
		if a, ok := it.(ap.Article); ok {
			return fnAp(i, a)
		}
		if a, ok := it.(*ap.Article); ok {
			return fnAp(i, *a)
		}
		if o, ok := it.(as.Object); ok {
			return fnAs(i, o)
		}
		if o, ok := it.(*as.Object); ok {
			return fnAs(i, *o)
		}
		return nil
	}
	loadFromObject := func(i *Item, a as.Object) error {
		title := jsonUnescape(a.Name.First().Value)

		i.Hash.FromActivityPub(a)
		if len(title) > 0 {
			i.Title = title
		}
		i.MimeType = MimeTypeHTML
		if a.Type == as.PageType {
			i.Data = string(a.URL.GetLink())
			i.MimeType = MimeTypeURL
		} else {
			if len(a.MediaType) > 0 {
				i.MimeType = MimeType(a.MediaType)
			}
			i.Data = jsonUnescape(a.Content.First().Value)
		}
		if !a.Published.IsZero() {
			i.SubmittedAt = a.Published
		}
		if !a.Updated.IsZero() {
			i.UpdatedAt = a.Updated
		}
		if i.Metadata == nil {
			i.Metadata = &ItemMetadata{}
		}

		if a.AttributedTo != nil {
			auth := Account{}
			auth.FromActivityPub(a.AttributedTo)
			i.SubmittedBy = &auth
			i.Metadata.AuthorURI = a.AttributedTo.GetLink().String()
		}
		if len(a.ID) > 0 {
			iri := a.GetLink()
			i.Metadata.ID = iri.String()
			i.Metadata.URL = a.URL.GetLink().String()
		}
		if a.Icon != nil {
			if a.Icon.IsObject() {
				if ic, ok := a.Icon.(*as.Object); ok {
					i.Metadata.Icon.MimeType = string(ic.MediaType)
					i.Metadata.Icon.URI = ic.URL.GetLink().String()
				}
				if ic, ok := a.Icon.(as.Object); ok {
					i.Metadata.Icon.MimeType = string(ic.MediaType)
					i.Metadata.Icon.URI = ic.URL.GetLink().String()
				}
			}
		}
		if a.InReplyTo != nil {
			par := Item{}
			par.FromActivityPub(a.InReplyTo)
			i.Parent = &par
		}
		if a.Context != nil {
			op := Item{}
			op.FromActivityPub(a.Context)
			i.OP = &op
		}
		if a.Tag != nil && len(a.Tag) > 0 {
			i.Metadata.Tags = make(TagCollection, 0)
			i.Metadata.Mentions = make(TagCollection, 0)

			tags := TagCollection{}
			tags.FromActivityPub(a.Tag)
			for _, t := range tags {
				if t.Name[0] == '#' {
					i.Metadata.Tags = append(i.Metadata.Tags, t)
				} else {
					i.Metadata.Mentions = append(i.Metadata.Mentions, t)
				}
			}
		}
		return nil
	}
	loadFromArticle := func(i *Item, a ap.Article) error {
		err := loadFromObject(i, a.Object.Parent)
		i.Score = a.Score
		// TODO(marius): here we seem to have a bug, when Source.Content is nil when it shouldn't
		//    to repro, I used some copy/pasted comments from console javascript
		if len(a.Source.Content) > 0 && len(a.Source.MediaType) > 0 {
			i.Data = jsonUnescape(a.Source.Content.First().Value)
			i.MimeType = MimeType(a.Source.MediaType)
		}
		return err
	}
	switch it.GetType() {
	case as.DeleteType:
		if act, ok := it.(*ap.Activity); ok {
			err := i.FromActivityPub(act.Object)
			i.Delete()
			return err
		}
		if act, ok := it.(ap.Activity); ok {
			err := i.FromActivityPub(act.Object)
			i.Delete()
			return err
		}
	case as.CreateType:
		fallthrough
	case as.UpdateType:
		fallthrough
	case as.ActivityType:
		if act, ok := it.(*ap.Activity); ok {
			err := i.FromActivityPub(act.Object)
			i.SubmittedBy.FromActivityPub(act.Actor)
			i.Metadata.AuthorURI = act.Actor.GetLink().String()
			return err
		}
		if act, ok := it.(ap.Activity); ok {
			err := i.FromActivityPub(act.Object)
			i.SubmittedBy.FromActivityPub(act.Actor)
			i.Metadata.AuthorURI = act.Actor.GetLink().String()
			return err
		}
	case as.ArticleType:
		fallthrough
	case as.NoteType:
		fallthrough
	case as.DocumentType:
		fallthrough
	case as.PageType:
		return articleFn(i, loadFromObject, loadFromArticle)
	case as.TombstoneType:
		id := it.GetLink()
		i.Hash.FromActivityPub(id)
		if i.Metadata == nil {
			i.Metadata = &ItemMetadata{}
		}
		if len(id) > 0 {
			i.Metadata.ID = id.String()
		}
		loadFromASObject := func(i *Item, o as.Object) error {
			if o.InReplyTo != nil {
				par := Item{}
				par.FromActivityPub(o.InReplyTo)
				i.Parent = &par
			}
			if o.Context != nil {
				op := Item{}
				op.FromActivityPub(o.Context)
				i.OP = &op
			}
			return nil
		}
		loadFromArticle := func(i *Item, a ap.Article) error {
			if a.InReplyTo != nil {
				par := Item{}
				par.FromActivityPub(a.InReplyTo)
				i.Parent = &par
			}
			if a.Context != nil {
				op := Item{}
				op.FromActivityPub(a.Context)
				i.OP = &op
			}
			return nil
		}
		articleFn(i, loadFromASObject, loadFromArticle)

		i.Flags = FlagsDeleted
		i.SubmittedBy = &AnonymousAccount
	default:
		return errors.Newf("invalid object type")
	}

	return nil
}

func (v *Vote) FromActivityPub(it as.Item) error {
	if it == nil {
		return errors.Newf("nil item received")
	}
	if it.IsLink() {
		return errors.Newf("unable to load from IRI")
	}
	switch it.GetType() {
	case as.UndoType:
		fallthrough
	case as.LikeType:
		fallthrough
	case as.DislikeType:
		fromAct := func (act ap.Activity, v *Vote) {
			on := Item{}
			on.FromActivityPub(act.Object)
			v.Item = &on

			er := Account{}
			er.FromActivityPub(act.Actor)
			v.SubmittedBy = &er

			v.SubmittedAt = act.Published
			v.UpdatedAt = act.Updated
			if act.Type == as.LikeType {
				v.Weight = 1
			}
			if act.Type == as.DislikeType {
				v.Weight = -1
			}
			if act.Type == as.UndoType {
				v.Weight = 0
			}
		}
		if act, ok := it.(ap.Activity); ok {
			fromAct(act, v)
		}
		if act, ok := it.(*ap.Activity); ok {
			fromAct(*act, v)
		}
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

func GetHashFromAP(obj as.Item) Hash {
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

//
//func getAccountHandle(o as.Item) string {
//	if o == nil {
//		return ""
//	}
//	i := o.(as.IRI)
//	s := strings.Split(string(i), "/")
//	return s[len(s)-1]
//}

func jsonUnescape(s string) string {
	var out []byte
	var err error
	if out, err = jsonparser.Unescape([]byte(s), nil); err != nil {
		Logger.Error(err.Error())
		return s
	}
	return string(out)
}

func (i *TagCollection) FromActivityPub(it as.ItemCollection) error {
	if it == nil || len(it) == 0 {
		return errors.Newf("empty collection")
	}
	for _, t := range it {
		if m, ok := t.(*as.Mention); ok {
			u := string(*t.GetID())
			// we have a link
			lt := Tag{
				URL:  u,
				Name: m.Name.First().Value,
			}
			*i = append(*i, lt)
		}
		if ob, ok := t.(*as.Object); ok {
			u := string(*t.GetID())
			// we have a link
			lt := Tag{
				URL:  u,
				Name: ob.Name.First().Value,
			}
			*i = append(*i, lt)
		}
	}
	return nil
}
