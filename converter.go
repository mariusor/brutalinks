package brutalinks

import (
	"encoding/base64"
	"net/url"
	"path/filepath"
	"strings"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/microcosm-cc/bluemonday"
)

type Converter interface {
	FromActivityPub(ob vocab.Item) error
}

func (h *Hash) FromActivityPub(it vocab.Item) error {
	if vocab.IsNil(it) {
		return nil
	}
	if it.GetLink() == vocab.PublicNS {
		*h = AnonymousHash
		return nil
	}
	*h = HashFromItem(it.GetLink())
	return nil
}

func FromObject(a *Account, o *vocab.Object) error {
	a.Hash.FromActivityPub(o)
	name := o.Name.First().Value
	if len(name) > 0 {
		a.Handle = name.String()
	}
	a.Flags = FlagsNone
	if a.Metadata == nil {
		a.Metadata = &AccountMetadata{}
	}
	if len(o.ID) > 0 {
		iri := o.GetLink()
		a.Metadata.ID = iri.String()
		if o.URL != nil {
			a.Metadata.URL = o.URL.GetLink().String()
		}
		if !HostIsLocal(a.Metadata.ID) {
			a.Metadata.Name = name.String()
		}
	}
	if o.Icon != nil {
		vocab.OnObject(o.Icon, func(o *vocab.Object) error {
			return iconMetadataFromObject(&a.Metadata.Icon, o)
		})
	}
	if o.GetType() == vocab.TombstoneType {
		a.Handle = Deleted
		a.Flags = a.Flags | FlagsDeleted
	}
	if !o.Published.IsZero() {
		a.CreatedAt = o.Published
	}
	if !o.Updated.IsZero() {
		a.UpdatedAt = o.Updated
	}
	if o.AttributedTo != nil {
		act := Account{}
		act.FromActivityPub(o.AttributedTo)
		a.CreatedBy = &act
	}
	return nil
}

func FromActor(a *Account, p *vocab.Actor) error {
	a.Hash.FromActivityPub(p)
	name := p.Name.First().Value
	if len(name) > 0 {
		a.Handle = name.String()
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
			a.Metadata.Name = name.String()
		}
	}
	switch p.GetType() {
	case vocab.TombstoneType:
		a.Handle = Deleted
		a.Flags = a.Flags | FlagsDeleted
	case vocab.GroupType:
		a.Flags = a.Flags | FlagsGroup
	case vocab.ApplicationType:
		a.Flags = a.Flags | FlagsApplication
	case vocab.ServiceType:
		a.Flags = a.Flags | FlagsService
	}
	if !p.Published.IsZero() {
		a.CreatedAt = p.Published
	}
	if !p.Updated.IsZero() {
		a.UpdatedAt = p.Updated
	}
	if p.AttributedTo != nil {
		act := Account{}
		act.FromActivityPub(p.AttributedTo)
		a.CreatedBy = &act
	}
	pName := p.PreferredUsername.First().Value
	if pName.Equals(vocab.Content("")) {
		pName = p.Name.First().Value
	}
	sum := p.Summary.First().Value
	if len(sum) > 0 {
		a.Metadata.Blurb = string(sum)
	}
	a.Handle = pName.String()
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
	if p.Icon != nil {
		vocab.OnObject(p.Icon, func(o *vocab.Object) error {
			return iconMetadataFromObject(&a.Metadata.Icon, o)
		})
	}
	if p.Icon != nil {
		vocab.OnObject(p.Icon, func(ic *vocab.Object) error {
			a.Metadata.Icon = ImageMetadata{
				MimeType: string(ic.MediaType),
			}
			if ic.Content != nil {
				a.Metadata.Icon.URI = ic.Content.First().String()
			} else if ic.URL != nil {
				a.Metadata.Icon.URI = ic.URL.GetLink().String()
			} else {
				a.Metadata.Icon.URI = ic.GetLink().String()
			}
			return nil
		})
	} else {
		a.Metadata.Icon = accountDefaultAvatar(a)
	}
	a.Metadata.Key = &SSHKey{Public: []byte(p.PublicKey.PublicKeyPem)}
	if p.Endpoints != nil {
		if p.Endpoints.OauthAuthorizationEndpoint != nil {
			u, _ := p.Endpoints.OauthAuthorizationEndpoint.GetLink().URL()
			a.Metadata.AuthorizationEndPoint = u.String()
		}
		if p.Endpoints.OauthTokenEndpoint != nil {
			u, _ := p.Endpoints.OauthTokenEndpoint.GetLink().URL()
			a.Metadata.TokenEndPoint = u.String()
		}
	}
	if p.Tag != nil {
		a.Metadata.Tags = make(TagCollection, 0)
		tags := TagCollection{}
		tags.FromActivityPub(p.Tag)
		for _, t := range tags {
			if t.Type == TagTag && !a.Metadata.Tags.Contains(t) {
				a.Metadata.Tags = append(a.Metadata.Tags, t)
			}
		}

		for _, t := range tags {
			if t.Name == tagNameSysOP && t.IsLocal() {
				a.Flags = a.Flags | FlagsOperator
			}
			if t.Name == tagNameModerator && t.IsLocal() {
				a.Flags = a.Flags | FlagsModerator
			}
			if !Instance.ModTags.Contains(t) {
				continue
			}
			for _, tt := range Instance.ModTags {
				if t.Metadata == nil || tt.Metadata == nil {
					continue
				}
				if tt.Name == tagNameSysOP && t.Metadata.ID == tt.Metadata.ID {
					a.Flags = a.Flags | FlagsOperator
				}
				if tt.Name == tagNameModerator && t.Metadata.ID == tt.Metadata.ID {
					a.Flags = a.Flags | FlagsModerator
				}
			}
		}
	}
	return nil
}

func (a *Account) FromActivityPub(it vocab.Item) error {
	if a == nil {
		return nil
	}
	if vocab.IsNil(it) {
		return errors.Newf("nil item received")
	}
	a.Pub = it
	if it.IsLink() {
		iri := it.GetLink()
		if iri == vocab.PublicNS {
			*a = AnonymousAccount
		}
		if iri.String() == Instance.Conf.APIURL {
			*a = SystemAccount
		}
		if !a.Hash.IsValid() {
			a.Hash.FromActivityPub(iri)
		}
		a.Metadata = &AccountMetadata{ID: iri.String()}
		return nil
	}
	switch it.GetType() {
	case vocab.IgnoreType, vocab.BlockType, vocab.FlagType:
		fallthrough
	case vocab.CreateType, vocab.UpdateType:
		return vocab.OnActivity(it, func(act *vocab.Activity) error {
			err := a.FromActivityPub(act.Object)
			if !a.CreatedBy.IsValid() {
				acc := Account{}
				acc.FromActivityPub(act.Actor)
				a.CreatedBy = &acc
			}
			return err
		})
	case vocab.TombstoneType:
		return vocab.OnObject(it, func(o *vocab.Object) error {
			return FromObject(a, o)
		})
	case vocab.ServiceType, vocab.GroupType, vocab.ApplicationType, vocab.OrganizationType, vocab.PersonType:
		return vocab.OnActor(it, func(p *vocab.Actor) error {
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

func FromObjectWithBinaryData(i *Item, a *vocab.Object) error {
	err := FromArticle(i, a)
	if err != nil {
		return err
	}
	return nil
}

func iconMetadataFromObject(m *ImageMetadata, o *vocab.Object) error {
	if m == nil || o == nil {
		return nil
	}
	m.MimeType = string(o.MediaType)
	if o.URL != nil {
		m.URI = o.URL.GetLink().String()
	}
	if o.Content != nil && len(o.Content) > 0 {
		var cnt []byte = o.Content.First().Value
		buf := make([]byte, base64.RawStdEncoding.DecodedLen(len(cnt)))
		if _, err := base64.RawStdEncoding.Decode(buf, cnt); err != nil {
			m.URI = base64.RawStdEncoding.EncodeToString(buf)
		} else {
			m.URI = string(cnt)
		}
	}
	return nil
}

func FromMention(t *Tag, a *vocab.Mention) error {
	t.Hash.FromActivityPub(a)
	if title := a.Name.First().Value; len(title) > 0 {
		t.Name = title.String()
	}
	t.Type = TagMention
	if t.Metadata == nil {
		t.Metadata = &ItemMetadata{}
	}

	if len(a.ID) > 0 {
		t.Metadata.ID = a.ID.String()
		t.Metadata.URL = a.ID.String()
	}
	if len(a.Href) > 0 {
		t.URL = a.Href.String()
	}
	return nil
}

func FromTag(t *Tag, a *vocab.Object) error {
	t.Hash.FromActivityPub(a)
	if title := a.Name.First().Value; len(title) > 0 {
		t.Name = title.String()
	}
	t.Type = TagTag
	if a.Type == vocab.MentionType {
		t.Type = TagMention
	}
	t.SubmittedAt = a.Published
	t.UpdatedAt = a.Updated
	if t.Metadata == nil {
		t.Metadata = &ItemMetadata{}
	}

	if a.AttributedTo != nil {
		auth := Account{Metadata: &AccountMetadata{}}
		auth.FromActivityPub(a.AttributedTo)
		t.SubmittedBy = &auth
		t.Metadata.AuthorURI = a.AttributedTo.GetLink().String()
	}
	if len(a.ID) > 0 {
		t.Metadata.ID = a.ID.String()
	}
	local := Instance.BaseURL
	local.Path = filepath.Join("t", strings.TrimPrefix(t.Name, "#"))
	t.URL = local.String()

	if a.Icon != nil {
		vocab.OnObject(a.Icon, func(o *vocab.Object) error {
			return iconMetadataFromObject(&t.Metadata.Icon, o)
		})
	}
	return nil
}

var LocalHTMLPolicy = BlueMondayPolicy()

func BlueMondayPolicy() *bluemonday.Policy {
	p := bluemonday.StrictPolicy()
	p.AllowStandardAttributes()
	p.AllowStandardURLs()
	p.AllowElements("section", "details")
	p.AllowElements("wbr")

	// The following are all inline phrasing elements
	p.AllowElements("abbr", "acronym", "cite", "dfn", "mark", "var")

	// "time" is permitted
	p.AllowAttrs("datetime").Matching(bluemonday.ISO8601).OnElements("time")

	// block and inline elements that impart no semantic meaning but style the
	// document
	p.AllowElements("small", "tt")

	// "bdi" "bdo" are permitted
	p.AllowAttrs("dir").Matching(bluemonday.Direction).OnElements("bdi", "bdo")
	return p
}

func FromArticle(i *Item, a *vocab.Object) error {
	i.Hash.FromActivityPub(a)
	if len(a.Name) > 0 {
		i.Title = a.Name.First().Value.String()
	}
	if len(a.Content) > 0 {
		i.MimeType = MimeTypeHTML
		if len(a.MediaType) > 0 {
			i.MimeType = string(a.MediaType)
		}
		i.Data = a.Content.First().Value.String()
	} else if a.URL != nil && len(a.URL.GetLink()) > 0 {
		i.Data = string(a.URL.GetLink())
		i.MimeType = MimeTypeURL
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
		vocab.OnObject(a.Icon, func(o *vocab.Object) error {
			return iconMetadataFromObject(&i.Metadata.Icon, o)
		})
	}
	if a.Context != nil {
		op := Item{}
		op.FromActivityPub(a.Context)
		i.OP = &op
	}
	if a.InReplyTo != nil {
		if repl, ok := a.InReplyTo.(vocab.ItemCollection); ok {
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
	if len(i.Title) == 0 && a.InReplyTo == nil {
		if a.Summary != nil && len(a.Summary) > 0 {
			i.Title = bluemonday.StrictPolicy().Sanitize(a.Summary.First().Value.String())
		}
	}
	// TODO(marius): here we seem to have a bug, when Source.Content is nil when it shouldn't
	//    to repro, I used some copy/pasted comments from console javascript
	if len(a.Source.Content) > 0 && len(a.Source.MediaType) > 0 {
		i.Data = LocalHTMLPolicy.Sanitize(a.Source.Content.First().Value.String())
		i.Data = a.Source.Content.First().Value.String()
		i.MimeType = string(a.Source.MediaType)
	}
	if a.Tag != nil {
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

func loadRecipientsFrom(recipients vocab.ItemCollection) (AccountCollection, bool) {
	result := make(AccountCollection, 0)
	isPublic := false
	for _, rec := range recipients {
		if rec == vocab.PublicNS {
			isPublic = true
			continue
		}
		_, maybeCol := vocab.Split(rec.GetLink())
		if vocab.ValidCollection(maybeCol) {
			continue
			if maybeCol != vocab.Followers && maybeCol != vocab.Following {
				// we don't know how to handle collections that don't contain accounts
				continue
			}
			acc := Account{
				Metadata: &AccountMetadata{
					ID: rec.GetLink().String(),
				},
			}
			result = append(result, acc)
		} else {
			acc := Account{}
			acc.FromActivityPub(rec)
			if acc.IsValid() {
				result = append(result, acc)
			}
		}
	}
	return result, isPublic
}

func loadRecipients(i *Item, it vocab.Item) error {
	i.MakePrivate()
	return vocab.OnObject(it, func(o *vocab.Object) error {
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

func (t *Tag) FromActivityPub(it vocab.Item) error {
	if vocab.IsNil(it) {
		return errors.Newf("nil tag received")
	}
	t.Pub = it
	typ := it.GetType()
	if it.IsLink() && typ != vocab.MentionType {
		t.Hash.FromActivityPub(it.GetLink())
		t.Type = TagTag
		t.Metadata = &ItemMetadata{
			ID: it.GetLink().String(),
		}
		return nil
	}
	switch typ {
	case vocab.DeleteType:
		return vocab.OnActivity(it, func(act *vocab.Activity) error {
			return t.FromActivityPub(act.Object)
		})
	case vocab.CreateType, vocab.UpdateType, vocab.ActivityType:
		return vocab.OnActivity(it, func(act *vocab.Activity) error {
			if (vocab.ActivityVocabularyTypes{vocab.CreateType, vocab.UpdateType}).Contains(act.Type) {
				return errors.Newf("Invalid activity to load from %s", act.Type)
			}
			if err := t.FromActivityPub(act.Object); err != nil {
				return err
			}
			t.SubmittedBy.FromActivityPub(act.Actor)
			if t.Metadata == nil {
				t.Metadata = &ItemMetadata{}
			}
			t.Metadata.AuthorURI = act.Actor.GetLink().String()
			return nil
		})
	case vocab.MentionType:
		return vocab.OnLink(it, func(m *vocab.Mention) error {
			return FromMention(t, m)
		})
	case vocab.TombstoneType:
		id := it.GetLink()
		t.Hash.FromActivityPub(id)
		t.Type = TagTag
		if t.Metadata == nil {
			t.Metadata = &ItemMetadata{}
		}
		if len(id) > 0 {
			t.Metadata.ID = id.String()
		}
		t.SubmittedBy = &AnonymousAccount
		vocab.OnTombstone(it, func(o *vocab.Tombstone) error {
			if o.FormerType == vocab.MentionType {
				t.Type = TagMention
			}
			return nil
		})
		vocab.OnObject(it, func(o *vocab.Object) error {
			t.SubmittedAt = o.Published
			t.UpdatedAt = o.Updated
			return nil
		})
	case vocab.ObjectType:
		fallthrough
	default:
		return vocab.OnObject(it, func(o *vocab.Object) error {
			return FromTag(t, o)
		})
	}
	return nil
}

func (i *Item) FromActivityPub(it vocab.Item) error {
	if vocab.IsNil(it) {
		return errors.Newf("nil item received")
	}
	i.Pub = it
	if it.IsLink() {
		i.Hash.FromActivityPub(it.GetLink())
		i.Metadata = &ItemMetadata{
			ID: it.GetLink().String(),
		}
		return nil
	}
	switch it.GetType() {
	case vocab.DeleteType:
		return vocab.OnActivity(it, func(act *vocab.Activity) error {
			err := i.FromActivityPub(act.Object)
			i.Delete()
			return err
		})
	case vocab.CreateType, vocab.UpdateType, vocab.ActivityType:
		return vocab.OnActivity(it, func(act *vocab.Activity) error {
			// TODO(marius): this logic is probably broken if the activity is anything else except a Create
			if !(vocab.ActivityVocabularyTypes{vocab.CreateType, vocab.UpdateType}).Contains(act.Type) {
				return errors.Newf("Invalid activity to load from %s", act.Type)
			}
			if err := i.FromActivityPub(act.Object); err != nil {
				return err
			}
			i.SubmittedBy.FromActivityPub(act.Actor)
			if i.Metadata == nil {
				i.Metadata = &ItemMetadata{}
			}
			i.Metadata.AuthorURI = act.Actor.GetLink().String()
			return loadRecipients(i, act)
		})
	case vocab.ArticleType, vocab.NoteType, vocab.DocumentType, vocab.PageType:
		return vocab.OnObject(it, func(a *vocab.Object) error {
			return FromArticle(i, a)
		})
	case vocab.ImageType, vocab.VideoType, vocab.AudioType:
		return vocab.OnObject(it, func(a *vocab.Object) error {
			return FromObjectWithBinaryData(i, a)
		})
	case vocab.TombstoneType:
		id := it.GetLink()
		i.Hash.FromActivityPub(id)
		if i.Metadata == nil {
			i.Metadata = &ItemMetadata{}
		}
		if len(id) > 0 {
			i.Metadata.ID = id.String()
		}
		vocab.OnObject(it, func(o *vocab.Object) error {
			if o.Context != nil {
				op := new(Item)
				if err := op.FromActivityPub(o.Context); err == nil {
					i.OP = op
				}
			}
			if o.InReplyTo != nil {
				if repl, ok := o.InReplyTo.(vocab.ItemCollection); ok {
					first := repl.First()
					if first != nil {
						par := new(Item)
						if err := par.FromActivityPub(first); err == nil {
							i.Parent = par
							if i.OP == nil {
								i.OP = par
							}
						}
					}
				} else {
					par := new(Item)
					if err := par.FromActivityPub(o.InReplyTo); err == nil {
						i.Parent = par
						if i.OP == nil {
							i.OP = par
						}
					}
				}
			}
			i.SubmittedAt = o.Published
			return nil
		})
		vocab.OnTombstone(it, func(t *vocab.Tombstone) error {
			i.UpdatedAt = t.Deleted
			if i.SubmittedAt.IsZero() {
				i.SubmittedAt = i.UpdatedAt
			}
			return nil
		})
		i.Delete()
		i.SubmittedBy = &AnonymousAccount
	default:
		return errors.Newf("invalid object type %q", it.GetType())
	}

	return nil
}

func (v *Vote) FromActivityPub(it vocab.Item) error {
	if vocab.IsNil(it) {
		return errors.Newf("nil item received")
	}
	v.Pub, _ = vocab.ToActivity(it)
	if it.IsLink() {
		return errors.Newf("unable to load from IRI")
	}
	switch it.GetType() {
	case vocab.UndoType, vocab.LikeType, vocab.DislikeType:
		fromAct := func(act vocab.Activity, v *Vote) {
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

			if act.Type == vocab.LikeType {
				v.Weight = 1
			}
			if act.Type == vocab.DislikeType {
				v.Weight = -1
			}
			if act.Type == vocab.UndoType {
				v.Weight = 0
				v.Metadata.OriginalIRI = act.Object.GetLink().String()
			}
		}
		vocab.OnActivity(it, func(act *vocab.Activity) error {
			fromAct(*act, v)
			return nil
		})
	}

	return nil
}

func HostIsLocal(s string) bool {
	if Instance.Conf == nil {
		return false
	}
	return host(s) == Instance.Conf.HostName || host(s) == host(Instance.Conf.APIURL)
}

func host(u string) string {
	if pu, err := url.Parse(u); err == nil {
		return pu.Host
	}
	return u
}

func (c *TagCollection) FromActivityPub(tag vocab.Item) error {
	if tag == nil {
		return errors.Newf("empty collection")
	}
	appendTag := func(it vocab.Item) error {
		t := Tag{}
		t.FromActivityPub(it)
		*c = append(*c, t)
		return nil
	}
	if tag.IsCollection() {
		return vocab.OnCollectionIntf(tag, func(c vocab.CollectionInterface) error {
			for _, it := range c.Collection() {
				appendTag(it)
			}
			return nil
		})
	} else {
		return appendTag(tag)
	}
}

func LoadFromActivityPubItem(it vocab.Item) (Renderable, error) {
	var (
		result Renderable
		err    error
		typ    = it.GetType()
	)
	if vocab.IsIRI(it) {
		item := new(Item)
		err = item.FromActivityPub(it)
		result = item
	}
	if typ == vocab.FollowType {
		f := new(FollowRequest)
		err = f.FromActivityPub(it)
		result = f
	}
	if typ == vocab.TombstoneType {
		vocab.OnTombstone(it, func(t *vocab.Tombstone) error {
			typ = t.FormerType
			return nil
		})
	}
	if ValidContentManagementTypes.Contains(typ) || typ == "" {
		item := new(Item)
		err = item.FromActivityPub(it)
		result = item
	}
	if ValidAppreciationTypes.Contains(typ) {
		vot := new(Vote)
		err = vot.FromActivityPub(it)
		result = vot
	}
	if ValidModerationActivityTypes.Contains(typ) {
		op := new(ModerationOp)
		err = op.FromActivityPub(it)
		result = op
	}
	if ValidActorTypes.Contains(typ) {
		acc := new(Account)
		err = acc.FromActivityPub(it)
		result = acc
	}
	if ValidContentTypes.Contains(typ) {
		item := new(Item)
		err = item.FromActivityPub(it)
		result = item
	}
	return result, err
}
