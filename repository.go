package brutalinks

import (
	"context"
	"encoding/base64"
	xerrors "errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "git.sr.ht/~mariusor/lw"
	"github.com/carlmjohnson/flowmatic"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/errors"
	j "github.com/go-ap/jsonld"
	"github.com/mariusor/qstring"
)

type repository struct {
	SelfURL string
	cache   cache
	app     *Account
	fedbox  *fedbox
	modTags TagCollection
	infoFn  CtxLogFn
	errFn   CtxLogFn
}

func (r repository) BaseURL() vocab.IRI {
	return r.fedbox.baseURL
}

func ActivityPubService(c appConfig) (*repository, error) {
	vocab.ItemTyperFunc = vocab.GetItemByType

	l := c.Logger.WithContext(log.Ctx{"log": "api"})
	infoFn := func(ctx ...log.Ctx) LogFn {
		return l.WithContext(ctx...).Debugf
	}
	errFn := func(ctx ...log.Ctx) LogFn {
		return l.WithContext(ctx...).Warnf
	}
	ua := fmt.Sprintf("%s-%s", c.HostName, Instance.Version)

	repo := &repository{
		SelfURL: c.BaseURL,
		infoFn:  infoFn,
		errFn:   errFn,
		cache:   caches(c.CachingEnabled),
	}
	var err error
	repo.fedbox, err = NewClient(
		WithURL(c.APIURL),
		WithLogger(c.Logger),
		WithUA(ua),
		SkipTLSCheck(!c.Env.IsProd()),
	)
	if err != nil {
		return repo, err
	}
	if c.CachingEnabled && c.Env.IsDev() {
		new(sync.Once).Do(func() {
			if err := WarmupCaches(repo, repo.fedbox.Service()); err != nil {
				c.Logger.WithContext(log.Ctx{"err": err.Error()}).Warnf("Unable to warmup cache")
			}
		})
	}

	if repo.app, err = AuthorizeOAuthClient(repo, c); err != nil {
		return repo, fmt.Errorf("failed to authenticate client: %w", err)
	}

	repo.modTags, err = SaveModeratorTags(repo)
	if err != nil {
		return repo, fmt.Errorf("failed to create mod tag objects: %w", err)
	}

	return repo, nil
}

func SaveModeratorTags(repo *repository) (TagCollection, error) {
	modTags := TagCollection{
		Tag{
			Type: TagTag,
			Name: tagNameModerator,
			URL:  repo.SelfURL + filepath.Join("/", "t", strings.TrimLeft(tagNameModerator, "#")),
		},
		Tag{
			Type: TagTag,
			Name: tagNameSysOP,
			URL:  repo.SelfURL + filepath.Join("/", "t", strings.TrimLeft(tagNameSysOP, "#")),
		},
	}
	tags, err := LoadModeratorTags(repo)
	if err != nil {
		repo.errFn()("unable to load moderation tags from remote instance: %s", err)
	}
	toSaveTags := make(TagCollection, 0)
	for _, mTag := range modTags {
		if !tags.Contains(mTag) {
			toSaveTags = append(toSaveTags, mTag)
		}
	}

	repo.fedbox.SignBy(repo.app)
	for i, tag := range toSaveTags {
		ap := buildAPTagObject(tag, repo)
		create := wrapItemInCreate(ap, repo.app.Pub)
		create.To, _, create.CC, create.BCC = repo.defaultRecipientsList(repo.app.Pub, true)
		_, tagIt, err := repo.fedbox.ToOutbox(context.TODO(), create)
		if err != nil {
			repo.errFn()("unable to save moderation tag %q on remote instance: %s", tag.Name, err)
		}
		if err = tag.FromActivityPub(tagIt); err != nil {
			repo.errFn()("unable to save moderation tag %q on remote instance: %s", tag.Name, err)
		}
		toSaveTags[i] = tag
	}
	return toSaveTags, nil
}

func buildAPTagObject(tag Tag, repo *repository) *vocab.Object {
	t := new(vocab.Object)
	t.AttributedTo = repo.app.Pub.GetLink()
	t.Name.Set(vocab.NilLangRef, vocab.Content(tag.Name))
	t.Summary.Set(vocab.NilLangRef, vocab.Content(fmt.Sprintf("Moderator tag for instance %s", repo.SelfURL)))
	t.URL = vocab.IRI(tag.URL)
	return t
}

func LoadModeratorTags(repo *repository) (TagCollection, error) {
	ff := &Filters{
		Name: CompStrs{EqualsString(tagNameModerator), EqualsString(tagNameSysOP)},
		//Type:   nilFilters, // TODO(marius): this seems to have a problem currently on FedBOX
		AttrTo: CompStrs{EqualsString(repo.app.AP().GetID().String())},
	}
	tags, _, err := repo.LoadTags(context.Background(), ff)
	return tags, err
}

func accountURL(acc Account) vocab.IRI {
	return vocab.IRI(fmt.Sprintf("%s%s", Instance.BaseURL.String(), AccountLocalLink(&acc)))
}

func BuildID(r Renderable) (vocab.ID, bool) {
	switch ob := r.(type) {
	case *Item:
		return BuildIDFromItem(*ob)
	}
	return "", false
}

func BuildIDFromItem(i Item) (vocab.ID, bool) {
	if !i.IsValid() {
		return "", false
	}
	if i.HasMetadata() && len(i.Metadata.ID) > 0 {
		return vocab.ID(i.Metadata.ID), true
	}
	return "", false
}

func GetID(a Renderable) vocab.ID {
	if !a.IsValid() {
		return vocab.PublicNS
	}
	return a.AP().GetID()
}

func appendRecipients(rec *vocab.ItemCollection, it vocab.Item) error {
	if vocab.IsNil(it) || rec == nil {
		return nil
	}
	if vocab.IsItemCollection(it) {
		return vocab.OnItemCollection(it, func(col *vocab.ItemCollection) error {
			for _, r := range *col {
				if !rec.Contains(r.GetLink()) {
					*rec = append(*rec, r.GetLink())
				}
			}
			return nil
		})
	}
	if !rec.Contains(it.GetLink()) {
		*rec = append(*rec, it.GetLink())
	}
	return nil
}

func appendReplies(parent vocab.Item) (vocab.ItemCollection, error) {
	if parent == nil {
		return nil, nil
	}
	repl := make(vocab.ItemCollection, 0)
	if parent.IsLink() {
		if !repl.Contains(parent.GetLink()) {
			repl = append(repl, parent.GetLink())
		}
		return repl, nil
	}
	err := vocab.OnObject(parent, func(ob *vocab.Object) error {
		if !repl.Contains(ob.GetLink()) {
			repl = append(repl, ob.GetLink())
		}
		if ob.InReplyTo == nil {
			return nil
		}
		if vocab.IsIRI(ob.InReplyTo) {
			if !repl.Contains(ob.InReplyTo.GetLink()) {
				repl = append(repl, ob.InReplyTo.GetLink())
				return nil
			}
		} else if vocab.IsObject(ob.InReplyTo) {
			return vocab.OnObject(ob.InReplyTo, func(r *vocab.Object) error {
				if !repl.Contains(r.GetLink()) {
					repl = append(repl, r.GetLink())
				}
				return nil
			})
		} else if vocab.IsItemCollection(ob.InReplyTo) {
			return vocab.OnCollectionIntf(ob.InReplyTo, func(col vocab.CollectionInterface) error {
				for _, r := range col.Collection() {
					if !repl.Contains(r.GetLink()) {
						repl = append(repl, r.GetLink())
					}
				}
				return nil
			})
		}
		return nil
	})
	return repl, err
}

func loadFromParent(ob *vocab.Object, it vocab.Item) error {
	if vocab.IsNil(it) || ob == nil {
		return nil
	}
	if repl, err := appendReplies(it); err == nil {
		ob.InReplyTo = repl
	}
	vocab.OnObject(it, func(p *vocab.Object) error {
		appendRecipients(&ob.To, p.To)
		appendRecipients(&ob.Bto, p.Bto)
		appendRecipients(&ob.CC, p.CC)
		appendRecipients(&ob.BCC, p.BCC)
		return nil
	})

	return nil
}

func (r *repository) loadAPItem(it vocab.Item, item Item) error {
	return vocab.OnObject(it, func(o *vocab.Object) error {
		if id, ok := BuildIDFromItem(item); ok {
			o.ID = id
		}
		if item.MimeType == MimeTypeURL {
			o.Type = vocab.PageType
			if item.Hash.IsValid() {
				o.URL = vocab.ItemCollection{
					vocab.IRI(item.Data),
					vocab.IRI(ItemLocalLink(&item)),
				}
			} else {
				o.URL = vocab.IRI(item.Data)
			}
		} else {
			wordCount := strings.Count(item.Data, " ") +
				strings.Count(item.Data, "\t") +
				strings.Count(item.Data, "\n") +
				strings.Count(item.Data, "\r\n")
			if wordCount > 300 {
				o.Type = vocab.ArticleType
			} else {
				o.Type = vocab.NoteType
			}

			if item.Hash.IsValid() {
				o.URL = vocab.IRI(ItemLocalLink(&item))
			}
			o.Name = make(vocab.NaturalLanguageValues, 0)
			switch item.MimeType {
			case MimeTypeMarkdown:
				o.Source.MediaType = vocab.MimeType(item.MimeType)
				o.MediaType = MimeTypeHTML
				if item.Data != "" {
					o.Source.Content.Set("en", vocab.Content(item.Data))
					o.Content.Set("en", vocab.Content(Markdown(item.Data)))
				}
			case MimeTypeText:
				fallthrough
			case MimeTypeHTML:
				o.MediaType = vocab.MimeType(item.MimeType)
				o.Content.Set("en", vocab.Content(item.Data))
			}
		}

		o.Published = item.SubmittedAt
		o.Updated = item.UpdatedAt

		if item.Deleted() {
			del := vocab.Tombstone{
				ID:         o.ID,
				Type:       vocab.TombstoneType,
				FormerType: o.Type,
				Deleted:    o.Updated,
			}
			repl := make(vocab.ItemCollection, 0)
			if item.Parent != nil {
				if par, ok := BuildID(item.Parent); ok {
					repl = append(repl, par)
				}
				if item.OP == nil {
					item.OP = item.Parent
				}
			}
			if item.OP != nil {
				if op, ok := BuildID(item.OP); ok {
					del.Context = op
					if !repl.Contains(op) {
						repl = append(repl, op)
					}
				}
			}
			if len(repl) > 0 {
				del.InReplyTo = repl
			}

			it = &del
			return nil
		}

		if item.Title != "" {
			o.Name.Set("en", vocab.Content(item.Title))
		}
		if item.SubmittedBy != nil {
			o.AttributedTo = GetID(item.SubmittedBy)
		}

		to, _, cc, bcc := r.defaultRecipientsList(nil, item.Public())
		repl := make(vocab.ItemCollection, 0)

		if item.Parent != nil {
			p := item.Parent
			first := true
			for {
				if par, ok := BuildID(p); ok {
					repl = append(repl, par)
				}
				par, ok := p.(*Item)
				if !ok {
					continue
				}
				if par.SubmittedBy.IsValid() {
					if pAuth := GetID(par.SubmittedBy); !vocab.PublicNS.Equals(pAuth, true) {
						if first {
							if !to.Contains(pAuth) {
								appendRecipients(&to, pAuth)
							}
							first = false
						} else if !cc.Contains(pAuth) {
							appendRecipients(&cc, pAuth)
						}
					}
				}
				if par.Parent == nil {
					break
				}
				p = par.Parent
			}
		}
		if item.OP != nil {
			if op, ok := BuildID(item.OP); ok {
				o.Context = op
			}
		}
		if len(repl) > 0 {
			o.InReplyTo = repl
		}

		if item.Metadata != nil {
			m := item.Metadata
			for _, rec := range m.To {
				mto := vocab.IRI(rec.Metadata.ID)
				if !to.Contains(mto) {
					appendRecipients(&to, mto)
				}
			}
			for _, rec := range m.CC {
				mcc := vocab.IRI(rec.Metadata.ID)
				if !cc.Contains(mcc) {
					appendRecipients(&cc, mcc)
				}
			}
			if m.Mentions != nil || m.Tags != nil {
				o.Tag = make(vocab.ItemCollection, 0)
				for _, men := range m.Mentions {
					// todo(marius): retrieve object ids of each mention and add it to the CC of the object
					t := vocab.Mention{
						Type: vocab.MentionType,
						Name: vocab.NaturalLanguageValues{{Ref: vocab.NilLangRef, Value: vocab.Content(men.Name)}},
						Href: vocab.IRI(men.URL),
					}
					if men.Metadata != nil && len(men.Metadata.ID) > 0 {
						t.ID = vocab.IRI(men.Metadata.ID)
					}
					o.Tag.Append(t)
				}
				for _, tag := range m.Tags {
					name := "#" + tag.Name
					if tag.Name[0] == '#' {
						name = tag.Name
					}
					t := vocab.Object{
						URL:  vocab.ID(tag.URL),
						Name: vocab.NaturalLanguageValues{{Ref: vocab.NilLangRef, Value: vocab.Content(name)}},
					}
					if tag.Metadata != nil && len(tag.Metadata.ID) > 0 {
						t.ID = vocab.IRI(tag.Metadata.ID)
					}
					o.Tag.Append(t)
				}
			}
		}
		o.To = to
		o.CC = cc
		o.BCC = bcc

		return nil
	})
}

var anonymousActor = &vocab.Actor{
	ID:                vocab.PublicNS,
	Name:              vocab.NaturalLanguageValues{{vocab.NilLangRef, vocab.Content(Anonymous)}},
	Type:              vocab.PersonType,
	PreferredUsername: vocab.NaturalLanguageValues{{vocab.NilLangRef, vocab.Content(Anonymous)}},
}

func anonymousPerson(url vocab.IRI) *vocab.Actor {
	p := anonymousActor
	p.Inbox = vocab.Inbox.IRI(url)
	return p
}

func (r *repository) loadAPPerson(a Account) *vocab.Actor {
	var p *vocab.Actor
	if act, ok := a.Pub.(*vocab.Actor); ok {
		p = act
	} else {
		p = new(vocab.Actor)
	}
	if p.Type == "" {
		p.Type = vocab.PersonType
	}

	if a.HasMetadata() {
		if p.Summary.Count() == 0 && len(a.Metadata.Blurb) > 0 {
			p.Summary = vocab.NaturalLanguageValuesNew()
			p.Summary.Set(vocab.NilLangRef, vocab.Content(a.Metadata.Blurb))
		}
		if p.Icon == nil && len(a.Metadata.Icon.URI) > 0 {
			avatar := vocab.ObjectNew(vocab.ImageType)
			avatar.MediaType = vocab.MimeType(a.Metadata.Icon.MimeType)
			avatar.URL = vocab.IRI(a.Metadata.Icon.URI)
			p.Icon = avatar
		}
	}

	if p.PreferredUsername.Count() == 0 {
		p.PreferredUsername = vocab.NaturalLanguageValuesNew()
		p.PreferredUsername.Set(vocab.NilLangRef, vocab.Content(a.Handle))
	}

	if a.Hash.IsValid() {
		if p.ID == "" {
			p.ID = vocab.ID(a.Metadata.ID)
		}
		if p.Name.Count() == 0 && a.Metadata.Name != "" {
			p.Name = vocab.NaturalLanguageValuesNew()
			p.Name.Set("en", vocab.Content(a.Metadata.Name))
		}
		if p.Inbox == nil && len(a.Metadata.InboxIRI) > 0 {
			p.Inbox = vocab.IRI(a.Metadata.InboxIRI)
		}
		if p.Outbox == nil && len(a.Metadata.OutboxIRI) > 0 {
			p.Outbox = vocab.IRI(a.Metadata.OutboxIRI)
		}
		if p.Liked == nil && len(a.Metadata.LikedIRI) > 0 {
			p.Liked = vocab.IRI(a.Metadata.LikedIRI)
		}
		if p.Followers == nil && len(a.Metadata.FollowersIRI) > 0 {
			p.Followers = vocab.IRI(a.Metadata.FollowersIRI)
		}
		if p.Following == nil && len(a.Metadata.FollowingIRI) > 0 {
			p.Following = vocab.IRI(a.Metadata.FollowingIRI)
		}
		if p.URL == nil && len(a.Metadata.URL) > 0 {
			p.URL = vocab.IRI(a.Metadata.URL)
		}
		if p.Endpoints == nil && r.fedbox.Service().Endpoints != nil {
			p.Endpoints = &vocab.Endpoints{
				SharedInbox:                r.fedbox.Service().Inbox,
				OauthAuthorizationEndpoint: r.fedbox.Service().Endpoints.OauthAuthorizationEndpoint,
				OauthTokenEndpoint:         r.fedbox.Service().Endpoints.OauthTokenEndpoint,
			}
		}
	}

	if p.PublicKey.ID == "" && a.IsValid() && a.HasMetadata() && a.Metadata.Key != nil && a.Metadata.Key.Public != nil {
		p.PublicKey = vocab.PublicKey{
			ID:           vocab.ID(fmt.Sprintf("%s#main-key", p.ID)),
			Owner:        p.ID,
			PublicKeyPem: fmt.Sprintf("-----BEGIN PUBLIC KEY-----\n%s\n-----END PUBLIC KEY-----", base64.StdEncoding.EncodeToString(a.Metadata.Key.Public)),
		}
	}
	return p
}

func (r *repository) WithAccount(a *Account) *repository {
	// TODO(marius): the decision which sign function to use (the one for S2S or the one for C2S)
	//   should be made in FedBOX, because that's the place where we know if the request we're signing
	//   is addressed to an IRI belonging to that specific FedBOX instance or to another ActivityPub server
	r.fedbox.SignBy(a)
	return r
}

func (r *repository) LoadItem(ctx context.Context, iri vocab.IRI) (Item, error) {
	var item Item
	art, err := r.fedbox.Object(ctx, iri)
	if err != nil {
		r.errFn()(err.Error())
		return item, err
	}
	if err = item.FromActivityPub(art); err == nil {
		var items ItemCollection
		items, err = r.loadItemsAuthors(ctx, item)
		items, err = r.loadItemsVotes(ctx, items...)
		if len(items) > 0 {
			item = items[0]
		}
	}
	return item, err
}

func hashesUnique(a Hashes) Hashes {
	u := make([]Hash, 0, len(a))
	m := make(map[string]bool)

	for _, val := range a {
		k := val.String()
		if _, ok := m[k]; !ok {
			m[k] = true
			u = append(u, val)
		}
	}
	return u
}

func (r *repository) loadAccountsFollowers(ctx context.Context, acc *Account) error {
	if !acc.HasMetadata() || len(acc.Metadata.FollowersIRI) == 0 || acc.AP() == nil {
		return nil
	}
	ac := acc.AP()
	f := &Filters{}
	searches := RemoteLoads{
		baseIRI(ac.GetLink()): []RemoteLoad{{actor: ac, loadFn: followers, filters: []*Filters{f}}},
	}
	return LoadFromSearches(ctx, r, searches, func(_ context.Context, c vocab.CollectionInterface, f *Filters) error {
		for _, fol := range c.Collection() {
			if !vocab.ActorTypes.Contains(fol.GetType()) {
				continue
			}
			p := new(Account)
			if err := p.FromActivityPub(fol); err == nil && p.IsValid() {
				acc.Followers = append(acc.Followers, *p)
			}
		}
		return nil
	})
}

func accountInCollection(ac Account, col AccountCollection) bool {
	for _, fol := range col {
		if fol.Hash == ac.Hash {
			return true
		}
	}
	return false
}

func (r *repository) loadAccountsFollowing(ctx context.Context, acc *Account) error {
	if !acc.HasMetadata() || len(acc.Metadata.FollowersIRI) == 0 || acc.AP() == nil {
		return nil
	}
	ac := acc.AP()
	f := &Filters{}
	searches := RemoteLoads{
		baseIRI(ac.GetLink()): []RemoteLoad{{actor: ac, loadFn: following, filters: []*Filters{f}}},
	}

	return LoadFromSearches(ctx, r, searches, func(_ context.Context, c vocab.CollectionInterface, f *Filters) error {
		for _, fol := range c.Collection() {
			if !vocab.ActorTypes.Contains(fol.GetType()) {
				continue
			}
			p := new(Account)
			if err := p.FromActivityPub(fol); err == nil && p.IsValid() {
				acc.Following = append(acc.Following, *p)
			}
		}
		return nil
	})
}

func getItemUpdatedTime(it vocab.Item) time.Time {
	var updated time.Time
	vocab.OnObject(it, func(ob *vocab.Object) error {
		updated = ob.Updated
		return nil
	})
	return updated
}

func (r *repository) loadAccountsOutbox(ctx context.Context, acc *Account) error {
	if !acc.HasMetadata() || len(acc.Metadata.OutboxIRI) == 0 || acc.AP() == nil {
		return nil
	}

	ac := acc.AP()
	f := Filters{
		Object:   derefIRIFilters,
		MaxItems: 300,
	}

	fa := f
	fa.Type = AppreciationActivitiesFilter
	fm := f
	fm.Type = ModerationActivitiesFilter
	fc := f
	fc.Type = CreateActivitiesFilter

	searches := RemoteLoads{
		baseIRI(ac.GetLink()): []RemoteLoad{
			{actor: ac, loadFn: outbox, filters: []*Filters{&fa, &fc, &fm}},
		},
	}
	return LoadFromSearches(ctx, r, searches, func(ctx context.Context, c vocab.CollectionInterface, f *Filters) error {
		var stop bool
		for _, it := range c.Collection() {
			if iri := it.GetLink(); !acc.Metadata.Outbox.Contains(iri) {
				acc.Metadata.Outbox = append(acc.Metadata.Outbox, vocab.FlattenProperties(it))
			}
			vocab.OnActivity(it, func(a *vocab.Activity) error {
				stop = a.Published.Sub(oneYearishAgo) < 0
				typ := it.GetType()
				if typ == vocab.CreateType {
					ob := a.Object
					if ob == nil {
						return nil
					}
					if ob.IsObject() {
						if ValidActorTypes.Contains(ob.GetType()) {
							act := Account{}
							act.FromActivityPub(a)
							acc.children = append(act.children, &act)
						}
					}
				}
				if ValidModerationActivityTypes.Contains(typ) {
					m := ModerationOp{}
					m.FromActivityPub(a)
					if m.Object != nil {
						if m.Object.Type() != ActorType {
							return nil
						}
						dude, ok := m.Object.(*Account)
						if !ok {
							return nil
						}
						if typ == vocab.BlockType {
							acc.Blocked = append(acc.Blocked, *dude)
						}
						if typ == vocab.IgnoreType {
							acc.Ignored = append(acc.Ignored, *dude)
						}
					}
				}
				return nil
			})
			if stop {
				return nil
			}
		}
		return nil
	})
}

func getRepliesOf(items ...Item) vocab.IRIs {
	repliesTo := make(vocab.IRIs, 0)
	iriFn := func(it Item) vocab.IRI {
		if it.Pub != nil {
			return it.Pub.GetLink()
		}
		if id, ok := BuildIDFromItem(it); ok {
			return id
		}
		return ""
	}
	for _, it := range items {
		if it.IsValid() && it.OP != nil && it.OP.IsValid() {
			it = *it.OP.(*Item)
		}
		if iri := iriFn(it); len(iri) > 0 && !repliesTo.Contains(iri) {
			repliesTo = append(repliesTo, iri)
		}
	}
	return repliesTo
}

func (r *repository) loadItemsReplies(ctx context.Context, items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return nil, nil
	}
	repliesTo := getRepliesOf(items...)
	if len(repliesTo) == 0 {
		return nil, nil
	}
	allReplies := make(ItemCollection, 0)
	f := Filters{
		Type:     ActivityTypesFilter(ValidContentTypes...),
		IRI:      CompStrs{notNilFilter},
		MaxItems: MaxContentItems,
	}

	searches := RemoteLoads{}
	for _, top := range repliesTo {
		base := baseIRI(top)
		searches[base] = append(searches[base], RemoteLoad{actor: top, loadFn: replies, filters: []*Filters{&f}})
	}
	err := LoadFromSearches(ctx, r, searches, func(_ context.Context, c vocab.CollectionInterface, f *Filters) error {
		for _, it := range c.Collection() {
			if !it.IsObject() {
				continue
			}
			i := new(Item)
			if err := i.FromActivityPub(it); err == nil && !allReplies.Contains(*i) {
				allReplies = append(allReplies, *i)
			}
		}
		return nil
	})
	if err != nil {
		r.errFn()(err.Error())
	}
	// TODO(marius): probably we can thread the replies right here
	return allReplies, nil
}

func likesFilter(iris vocab.IRIs) RemoteLoads {
	ff := Filters{
		Type:     ActivityTypesFilter(ValidAppreciationTypes...),
		MaxItems: 500,
	}
	load := RemoteLoad{
		loadFn:  likes,
		filters: []*Filters{},
	}
	searches := RemoteLoads{}
	for _, iri := range iris {
		l := load
		l.actor = iri
		f := ff
		l.filters = append(l.filters, &f)
		base := baseIRI(iri)
		if _, ok := searches[base]; !ok {
			searches[base] = make([]RemoteLoad, 0)
		}
		searches[base] = append(searches[base], l)
	}
	return searches
}

func mixedLikesFilter(iris vocab.IRIs) RemoteLoads {
	g := make(map[vocab.IRI]vocab.IRIs)
	for _, iri := range iris {
		base := baseIRI(iri)
		if _, ok := g[base]; !ok {
			g[base] = make(vocab.IRIs, 0)
		}
		g[base] = append(g[base], iri)
	}
	ff := Filters{
		Type:     ActivityTypesFilter(ValidAppreciationTypes...),
		MaxItems: MaxContentItems * 100,
	}
	load := RemoteLoad{
		actor:   Instance.front.storage.app.Pub,
		loadFn:  inbox,
		filters: []*Filters{},
	}
	searches := RemoteLoads{}
	for b, iris := range g {
		l := load
		f := ff
		f.Object = &Filters{
			IRI: IRIsFilter(iris...),
		}
		l.filters = append(l.filters, &f)
		searches[b] = append(searches[b], l)
	}
	return searches
}

func irisFromItems(items ...Item) vocab.IRIs {
	iris := make(vocab.IRIs, 0)
	for _, it := range items {
		if it.Deleted() {
			continue
		}
		iris = append(iris, it.Pub.GetLink())
	}
	return iris
}

func (r *repository) loadItemsVotes(ctx context.Context, items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}

	var searches RemoteLoads
	if Instance.Conf.DownvotingEnabled {
		searches = mixedLikesFilter(irisFromItems(items...))
	} else {
		searches = likesFilter(irisFromItems(items...))
	}
	votes := make(VoteCollection, 0)
	err := LoadFromSearches(ctx, r, searches, func(_ context.Context, c vocab.CollectionInterface, f *Filters) error {
		for _, vAct := range c.Collection() {
			if !vAct.IsObject() || !ValidAppreciationTypes.Contains(vAct.GetType()) {
				continue
			}
			v := Vote{}
			if err := v.FromActivityPub(vAct); err == nil && !votes.Contains(v) {
				votes = append(votes, v)
			}
		}
		return nil
	})
	for k, ob := range items {
		for _, v := range votes {
			if v.Item == nil {
				continue
			}
			if items[k].Votes == nil {
				items[k].Votes = make(VoteCollection, 0)
			}
			if itemsEqual(*v.Item, ob) && !items[k].Votes.Contains(v) {
				items[k].Votes = append(items[k].Votes, v)
			}
		}
	}
	return items, err
}

func EqualsString(s string) CompStr {
	return CompStr{Operator: "=", Str: s}
}

func ItemIRIFilter(items ...Item) CompStrs {
	filter := make(CompStrs, 0)
	for _, it := range items {
		hash := EqualsString(it.Metadata.ID)
		if len(hash.Str) == 0 || filter.Contains(hash) {
			continue
		}
		filter = append(filter, hash)
	}
	return filter
}

func AccountsIRIFilter(accounts ...Account) CompStrs {
	filter := make(CompStrs, 0)
	for _, ac := range accounts {
		if ac.Pub == nil {
			continue
		}
		f := EqualsString(ac.Pub.GetLink().String())
		if filter.Contains(f) {
			continue
		}
		filter = append(filter, f)
	}
	return filter
}

func ActivityTypesFilter(t ...vocab.ActivityVocabularyType) CompStrs {
	r := make(CompStrs, len(t))
	for i, typ := range t {
		r[i] = EqualsString(string(typ))
	}
	return r
}

func (r *repository) loadAccountsAuthors(ctx context.Context, accounts ...Account) (AccountCollection, error) {
	if len(accounts) == 0 {
		return accounts, nil
	}
	fActors := Filters{}
	creators := make(AccountCollection, 0)
	for _, ac := range accounts {
		if ac.CreatedBy == nil {
			continue
		}
		creators = append(creators, *ac.CreatedBy)
	}
	if len(creators) == 0 {
		return accounts, nil
	}
	fActors.IRI = AccountsIRIFilter(creators...)
	var authors AccountCollection
	if len(fActors.IRI) > 0 {
		var err error
		authors, err = r.accounts(ctx, &fActors)
		if err != nil {
			return accounts, errors.Annotatef(err, "unable to load accounts authors")
		}
	}
	for k, ac := range accounts {
		found := false
		for i, auth := range authors {
			if !auth.IsValid() {
				continue
			}
			if renderablesEqual(ac.CreatedBy, &auth) {
				accounts[k].CreatedBy = &authors[i]
				found = true
			}
		}
		if !found {
			accounts[k].CreatedBy = &SystemAccount
		}
	}
	return accounts, nil
}

func (r *repository) loadFollowsAuthors(ctx context.Context, items ...FollowRequest) ([]FollowRequest, error) {
	if len(items) == 0 {
		return items, nil
	}
	submitters := make(AccountCollection, 0)
	remoteSubmitters := make(AccountCollection, 0)
	for _, it := range items {
		if it.SubmittedBy == nil {
			continue
		}
		if sub := *it.SubmittedBy; sub.IsValid() {
			if sub.IsLocal() && !submitters.Contains(sub) {
				submitters = append(submitters, sub)
			}
			if sub.IsFederated() && !remoteSubmitters.Contains(sub) {
				remoteSubmitters = append(remoteSubmitters, sub)
			}
		}
	}
	fActors := Filters{
		IRI:   AccountsIRIFilter(submitters...),
		Actor: derefIRIFilters,
	}

	var authors AccountCollection
	if len(fActors.IRI) > 0 {
		authors, _ = r.accounts(ctx, &fActors)
	}
	for _, remoteAcc := range remoteSubmitters {
		it, err := r.fedbox.Actor(ctx, remoteAcc.AP().GetLink())
		if err != nil {
			continue
		}
		remoteAcc.FromActivityPub(it)
		if !authors.Contains(remoteAcc) {
			authors = append(authors, remoteAcc)
		}
	}
	for k, it := range items {
		for i, auth := range authors {
			if !auth.IsValid() {
				continue
			}
			if accountsEqual(*it.SubmittedBy, auth) {
				items[k].SubmittedBy = &authors[i]
			}
		}
	}
	return items, nil
}

func (r *repository) loadModerationFollowups(ctx context.Context, items RenderableList) ([]ModerationOp, error) {
	inReplyTo := make(vocab.IRIs, 0)
	for _, it := range items {
		iri := it.AP().GetLink()
		if !inReplyTo.Contains(iri) {
			inReplyTo = append(inReplyTo, iri)
		}
	}

	modActions := new(Filters)
	modActions.Type = ActivityTypesFilter(vocab.DeleteType, vocab.UpdateType)
	modActions.InReplTo = IRIsFilter(inReplyTo...)
	modActions.Actor = derefIRIFilters
	modActions.Object = derefIRIFilters
	act, err := r.fedbox.Outbox(ctx, r.app.Pub, Values(modActions))
	if err != nil {
		return nil, err
	}
	modFollowups := make(ModerationRequests, 0)
	err = vocab.OnCollectionIntf(act, func(c vocab.CollectionInterface) error {
		for _, it := range c.Collection() {
			m := new(ModerationOp)
			if err := m.FromActivityPub(it); err != nil {
				continue
			}
			if !modFollowups.Contains(*m) {
				modFollowups = append(modFollowups, *m)
			}
		}
		return nil
	})
	return modFollowups, err
}

func ModerationSubmittedByHashFilter(items ...ModerationOp) CompStrs {
	accounts := make(AccountCollection, 0)
	for _, it := range items {
		if !it.SubmittedBy.IsValid() || accounts.Contains(*it.SubmittedBy) {
			continue
		}
		accounts = append(accounts, *it.SubmittedBy)
	}
	return AccountsIRIFilter(accounts...)
}

func (r *repository) loadModerationDetails(ctx context.Context, items ...ModerationOp) ([]ModerationOp, error) {
	if len(items) == 0 {
		return items, nil
	}
	fActors := new(Filters)
	fObjects := new(Filters)
	fActors.IRI = ModerationSubmittedByHashFilter(items...)
	fObjects.IRI = make(CompStrs, 0)
	for _, it := range items {
		if it.Object == nil || it.Object.AP() == nil {
			continue
		}
		_, hash := filepath.Split(it.Object.AP().GetLink().String())
		switch it.Object.Type() {
		case ActorType:
			hash := LikeString(hash)
			if !fActors.IRI.Contains(hash) {
				fActors.IRI = append(fActors.IRI, hash)
			}
		case CommentType:
			hash := LikeString(hash)
			if !fObjects.IRI.Contains(hash) {
				fObjects.IRI = append(fObjects.IRI, hash)
			}
		}
	}

	if len(fActors.IRI) == 0 {
		return items, errors.Errorf("unable to load items authors")
	}
	authors, err := r.accounts(ctx, fActors)
	if err != nil {
		return items, errors.Annotatef(err, "unable to load items authors")
	}
	var objects ItemCollection
	if len(fObjects.IRI) > 0 {
		if objects, err = r.objects(ctx, fObjects); err != nil {
			return items, errors.Annotatef(err, "unable to load items objects")
		}
	}
	for k, g := range items {
		if it, ok := g.Object.(*ModerationOp); ok {
			for i, auth := range authors {
				if accountsEqual(*it.SubmittedBy, auth) {
					it.SubmittedBy = &authors[i]
				}
				if it.Object != nil && it.Object.AP().GetLink().Equals(auth.AP().GetLink(), false) {
					it.Object = &authors[i]
				}
			}
			for i, obj := range objects {
				if it.Object != nil && it.Object.AP().GetLink().Equals(obj.AP().GetLink(), false) {
					it.Object = &(objects[i])
				}
			}
			items[k].Object = it
		}
		if it, ok := g.Object.(*Account); ok {
			for i, auth := range authors {
				if accountsEqual(*it, auth) {
					it = &authors[i]
				}
			}
			items[k].Object = it
		}
		if it, ok := g.Object.(*Item); ok {
			for i, obj := range objects {
				if itemsEqual(*it, obj) {
					it = &objects[i]
				}
			}
			for i, auth := range authors {
				if it.SubmittedBy != nil && accountsEqual(*it.SubmittedBy, auth) {
					it.SubmittedBy = &authors[i]
				}
			}
			items[k].Object = it
		}
	}
	return items, nil
}

func renderablesEqual(r1, r2 Renderable) bool {
	return r1.ID() == r2.ID()
}
func itemsEqual(i1, i2 Item) bool {
	return i1.Hash == i2.Hash
}

func accountsEqual(a1, a2 Account) bool {
	return a1.Hash == a2.Hash || (len(a1.Handle)+len(a2.Handle) > 0 && a1.Handle == a2.Handle)
}

func baseIRI(iri vocab.IRI) vocab.IRI {
	u, _ := iri.URL()
	u.Path = ""
	return vocab.IRI(u.String())
}

func (r *repository) loadItemsAuthors(ctx context.Context, items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}

	accounts := make(vocab.IRIs, 0)
	for _, it := range items {
		if it.SubmittedBy.IsValid() {
			if !accounts.Contains(it.SubmittedBy.AP().GetLink()) {
				// Adding an item's author to the list of accounts we want to load from the ActivityPub API
				accounts = append(accounts, it.SubmittedBy.AP().GetLink())
			}
		}
		if it.HasMetadata() {
			// Adding an item's recipients list (To and CC) to the list of accounts we want to load from the ActivityPub API
			for _, to := range it.Metadata.To {
				if to.AP() == nil {
					continue
				}
				if !accounts.Contains(to.AP().GetLink()) {
					accounts = append(accounts, to.AP().GetLink())
				}
			}
			for _, cc := range it.Metadata.CC {
				if cc.AP() == nil {
					continue
				}
				if !accounts.Contains(cc.AP().GetLink()) {
					accounts = append(accounts, cc.AP().GetLink())
				}
			}
		}
		for _, com := range *it.Children() {
			if ob, ok := com.(*Item); ok {
				if auth := ob.SubmittedBy; !auth.IsValid() {
					if !accounts.Contains(auth.AP().GetLink()) {
						accounts = append(accounts, auth.AP().GetLink())
					}
				}

			}
		}
	}

	if len(accounts) == 0 {
		return items, nil
	}

	authors := make(AccountCollection, 0)
	for _, auth := range accounts {
		it, err := r.fedbox.client.Actor(ctx, auth.GetLink())
		if err != nil {
			r.errFn(log.Ctx{"type": fmt.Sprintf("%T", it)})(err.Error())
			continue
		}
		acc := Account{}
		if err := acc.FromActivityPub(it); err != nil {
			r.errFn(log.Ctx{"type": fmt.Sprintf("%T", it)})(err.Error())
			continue
		}
		if acc.IsValid() && !authors.Contains(acc) {
			authors = append(authors, acc)
		}
	}
	col := make(ItemCollection, 0)
	for _, it := range items {
		for a := range authors {
			auth := authors[a]
			if !auth.IsValid() || auth.Handle == "" {
				continue
			}
			if it.SubmittedBy.IsValid() && it.SubmittedBy.Hash == auth.Hash {
				it.SubmittedBy = &auth
			}
			if it.UpdatedBy.IsValid() && it.UpdatedBy.Hash == auth.Hash {
				it.UpdatedBy = &auth
			}
			if !it.HasMetadata() {
				continue
			}
			for i, to := range it.Metadata.To {
				if to.IsValid() && to.Hash == auth.Hash {
					it.Metadata.To[i] = auth
				}
			}
			for i, cc := range it.Metadata.CC {
				if cc.IsValid() && cc.Hash == auth.Hash {
					it.Metadata.CC[i] = auth
				}
			}
			for i, com := range it.children {
				if ob, ok := com.(*Item); ok {
					if com.IsValid() && ob.SubmittedBy.Hash == auth.Hash {
						ob.SubmittedBy = &auth
						it.children[i] = ob
					}
				}
			}
		}
		col = append(col, it)
	}
	return col, nil
}

func getCollectionPrevNext(col vocab.CollectionInterface) (prev, next string) {
	qFn := func(i vocab.Item) url.Values {
		if i == nil {
			return url.Values{}
		}
		if u, err := i.GetLink().URL(); err == nil {
			return u.Query()
		}
		return url.Values{}
	}
	beforeFn := func(i vocab.Item) string {
		return qFn(i).Get("before")
	}
	afterFn := func(i vocab.Item) string {
		return qFn(i).Get("after")
	}
	nextFromLastFn := func(i vocab.Item) string {
		if u, err := i.GetLink().URL(); err == nil {
			_, next = filepath.Split(u.Path)
			return next
		}
		return ""
	}
	switch col.GetType() {
	case vocab.OrderedCollectionPageType:
		if c, ok := col.(*vocab.OrderedCollectionPage); ok {
			prev = beforeFn(c.Prev)
			if int(c.TotalItems) > len(c.OrderedItems) {
				next = afterFn(c.Next)
			}
		}
	case vocab.OrderedCollectionType:
		if c, ok := col.(*vocab.OrderedCollection); ok {
			if len(c.OrderedItems) > 0 && int(c.TotalItems) > len(c.OrderedItems) {
				next = nextFromLastFn(c.OrderedItems[len(c.OrderedItems)-1])
			}
		}
	case vocab.CollectionPageType:
		if c, ok := col.(*vocab.CollectionPage); ok {
			prev = beforeFn(c.Prev)
			if int(c.TotalItems) > len(c.Items) {
				next = afterFn(c.Next)
			}
		}
	case vocab.CollectionType:
		if c, ok := col.(*vocab.Collection); ok {
			if len(c.Items) > 0 && int(c.TotalItems) > len(c.Items) {
				next = nextFromLastFn(c.Items[len(c.Items)-1])
			}
		}
	}
	// NOTE(marius): we check if current Collection id contains a cursor, and if `after` points to the same URL
	//   we don't take it into consideration.
	if next != "" {
		f := struct {
			Next string `qstring:"after"`
		}{}
		if err := qstring.Unmarshal(qFn(col.GetLink()), &f); err == nil && next == f.Next {
			next = ""
		}
	}
	return prev, next
}

func (r *repository) account(ctx context.Context, ff *Filters) (*Account, error) {
	accounts, err := r.accounts(ctx, ff)
	if err != nil {
		return &AnonymousAccount, err
	}
	if len(accounts) == 0 {
		return &AnonymousAccount, errors.NotFoundf("account not found")
	}
	if len(accounts) > 1 {
		return &AnonymousAccount, errors.BadRequestf("too many accounts found")
	}
	return &accounts[0], nil
}

func accumulateAccountsFromCollection(col vocab.CollectionInterface) (AccountCollection, CompStrs, error) {
	accounts := make(AccountCollection, 0)
	deferredTagLoads := make(CompStrs, 0)
	for _, it := range col.Collection() {
		if !it.IsObject() || !ValidActorTypes.Contains(it.GetType()) {
			continue
		}
		a := Account{}
		if err := a.FromActivityPub(it); err == nil && a.IsValid() {
			if len(a.Metadata.Tags) > 0 && deferredTagLoads != nil {
				for _, t := range a.Metadata.Tags {
					if t.Name == "" && t.Metadata.ID != "" {
						deferredTagLoads = append(deferredTagLoads, EqualsString(t.Metadata.ID))
					}
				}
			}
			accounts = append(accounts, a)
		}
	}
	return accounts, deferredTagLoads, nil
}

func assignTagsToAccounts(accounts AccountCollection, col vocab.CollectionInterface) error {
	for _, it := range col.Collection() {
		for _, a := range accounts {
			for i, t := range a.Metadata.Tags {
				if it.GetID().Equals(vocab.IRI(t.Metadata.ID), true) {
					tt := Tag{}
					if err := tt.FromActivityPub(it); err == nil && !a.Metadata.Tags.Contains(tt) {
						a.Metadata.Tags[i] = tt
					}
				}
			}
		}
	}
	return nil
}

func (r *repository) accountsFromRemote(ctx context.Context, remote vocab.Item, ff ...*Filters) (AccountCollection, error) {
	accounts := make(AccountCollection, 0)
	localBase := baseIRI(r.fedbox.Service().GetLink())
	isRemote := remote != nil && !remote.GetLink().Contains(localBase, true)
	searches := RemoteLoads{}
	if isRemote {
		searches[remote.GetLink()] = []RemoteLoad{{actor: remote, loadFn: colIRI(actors), filters: ff}}
	} else {
		searches[localBase] = []RemoteLoad{{actor: r.fedbox.Service(), loadFn: colIRI(actors), filters: ff}}
	}
	deferredTagLoads := make(CompStrs, 0)
	err := LoadFromSearches(ctx, r, searches, func(_ context.Context, col vocab.CollectionInterface, f *Filters) error {
		// TODO(marius): this needs to be externalized also to a different function that we can pass from outer scope
		//   This function implements the logic for breaking out of the collection iteration cycle and returns a bool
		acc, tags, err := accumulateAccountsFromCollection(col)
		accounts = append(accounts, acc...)
		deferredTagLoads = append(deferredTagLoads, tags...)
		return err
	})
	if err != nil {
		return accounts, err
	}
	tagSearches := RemoteLoads{
		localBase: []RemoteLoad{{
			actor:   r.fedbox.Service(),
			loadFn:  colIRI(objects),
			filters: []*Filters{{IRI: deferredTagLoads}},
		}},
	}
	if isRemote {
		tagSearches[remote.GetLink()] = []RemoteLoad{{
			actor:   remote,
			loadFn:  colIRI(objects),
			filters: []*Filters{{IRI: deferredTagLoads}},
		}}
	}
	return accounts, LoadFromSearches(ctx, r, tagSearches, func(_ context.Context, col vocab.CollectionInterface, f *Filters) error {
		return assignTagsToAccounts(accounts, col)
	})
}

func (r *repository) accounts(ctx context.Context, ff ...*Filters) (AccountCollection, error) {
	return r.accountsFromRemote(ctx, r.fedbox.Service().GetLink(), ff...)
}

func (r *repository) objects(ctx context.Context, ff ...*Filters) (ItemCollection, error) {
	searches := RemoteLoads{
		r.fedbox.Service().GetLink(): []RemoteLoad{{actor: r.fedbox.Service(), loadFn: colIRI(objects), filters: ff}},
	}

	items := make(ItemCollection, 0)
	err := LoadFromSearches(ctx, r, searches, func(_ context.Context, c vocab.CollectionInterface, f *Filters) error {
		for _, it := range c.Collection() {
			i := new(Item)
			if err := i.FromActivityPub(it); err == nil && i.IsValid() {
				items = append(items, *i)
			}
		}
		return nil
	})
	if err != nil {
		return items, err
	}
	items, _ = r.loadItemsAuthors(ctx, items...)
	items, _ = r.loadItemsVotes(ctx, items...)
	return items, nil
}

func validFederated(i Item, f *Filters) bool {
	ob, err := vocab.ToObject(i.Pub)
	if err != nil {
		return false
	}
	if len(f.Generator) > 0 {
		for _, g := range f.Generator {
			if i.Pub == nil || ob.Generator == nil {
				continue
			}
			if g == nilFilter {
				if ob.Generator.GetLink().Equals(vocab.IRI(Instance.BaseURL.String()), false) {
					return false
				}
				return true
			}
			if ob.Generator.GetLink().Equals(vocab.IRI(g.Str), false) {
				return true
			}
		}
	}
	// @todo(marius): currently this marks as valid nil generator, but we eventually want non nil generators
	return ob != nil && ob.Generator == nil
}

func validRecipients(i Item, f *Filters) bool {
	if len(f.Recipients) > 0 {
		for _, r := range f.Recipients {
			if vocab.IRI(r.Str).Equals(vocab.PublicNS, false) && i.Private() {
				return false
			}
		}
	}
	return true
}

func validItem(it Item, f *Filters) bool {
	if keep := validRecipients(it, f); !keep {
		return keep
	}
	if keep := validFederated(it, f); !keep {
		return keep
	}
	return true
}

func filterItems(items ItemCollection, f *Filters) ItemCollection {
	result := make(ItemCollection, 0)
	for _, it := range items {
		if !it.HasMetadata() {
			continue
		}
		if validItem(it, f) {
			result = append(result, it)
		}
	}

	return result
}

func IRIsLikeFilter(iris ...vocab.IRI) CompStrs {
	r := make(CompStrs, len(iris))
	for i, iri := range iris {
		r[i] = LikeString(iri.String())
	}
	return r
}

func IRIsFilter(iris ...vocab.IRI) CompStrs {
	r := make(CompStrs, len(iris))
	for i, iri := range iris {
		r[i] = EqualsString(iri.String())
	}
	return r
}

func appendToIRIs(iris *vocab.IRIs, props ...vocab.Item) {
	append := func(iris *vocab.IRIs, prop vocab.Item) {
		if vocab.IsNil(prop) {
			return
		}
		iri := prop.GetLink()
		if iri == "" || iri == vocab.PublicNS {
			return
		}
		if _, col := vocab.Split(iri); vocab.ValidActivityCollection(col) || iris.Contains(iri) {
			return
		}
		*iris = append(*iris, iri)
	}
	for _, prop := range props {
		if vocab.IsObject(prop) {
			continue
		}
		if vocab.IsItemCollection(prop) {
			vocab.OnItemCollection(prop, func(col *vocab.ItemCollection) error {
				for _, it := range col.Collection() {
					append(iris, it)
				}
				return nil
			})
		} else {
			append(iris, prop)
		}
	}
}

func accumulateItemIRIs(it vocab.Item, deps deps) vocab.IRIs {
	if vocab.IsNil(it) {
		return nil
	}
	if vocab.IsIRI(it) {
		return vocab.IRIs{it.GetLink()}
	}
	iris := make(vocab.IRIs, 0)

	vocab.OnObject(it, func(o *vocab.Object) error {
		if deps.Authors {
			appendToIRIs(&iris, o.AttributedTo)
		}
		if deps.Replies {
			appendToIRIs(&iris, o.AttributedTo)
		}
		appendToIRIs(&iris, o.AttributedTo, o.InReplyTo)
		return nil
	})
	if withRecipients, ok := it.(vocab.HasRecipients); ok {
		appendToIRIs(&iris, withRecipients.Recipients())
	}
	return iris
}

// LoadSearches loads all elements from RemoteLoads
// Iterating over the activities in the resulting collections, we gather the objects and accounts
func (r *repository) LoadSearches(ctx context.Context, searches RemoteLoads, deps deps) (Cursor, error) {
	items := make(ItemCollection, 0)
	follows := make(FollowRequests, 0)
	accounts := make(AccountCollection, 0)
	moderations := make(ModerationRequests, 0)
	appreciations := make(VoteCollection, 0)
	relations := sync.Map{}

	deferredRemote := make(vocab.IRIs, 0)

	result := make(RenderableList, 0)
	resM := new(sync.RWMutex)

	var next, prev string
	err := LoadFromSearches(ctx, r, searches, func(ctx context.Context, col vocab.CollectionInterface, f *Filters) error {
		if len(col.Collection()) > 0 {
			prev, next = getCollectionPrevNext(col)
		}
		r.infoFn(log.Ctx{"col": col.GetID()})("loading")
		for _, it := range col.Collection() {
			err := vocab.OnActivity(it, func(a *vocab.Activity) error {
				typ := it.GetType()
				if typ == vocab.CreateType {
					ob := a.Object
					if ob == nil {
						return errors.Newf("nil activity object")
					}
					if vocab.IsObject(ob) {
						if ValidContentTypes.Contains(ob.GetType()) {
							i := Item{}
							if err := i.FromActivityPub(a); err != nil {
								return err
							}
							if validItem(i, f) {
								items = append(items, i)
							}
						}
						if ValidActorTypes.Contains(ob.GetType()) {
							act := Account{}
							if err := act.FromActivityPub(a); err != nil {
								return err
							}
							accounts = append(accounts, act)
						}
					} else {
						i := Item{}
						if err := i.FromActivityPub(a); err != nil {
							return err
						}
					}
					relations.Store(a.GetLink(), ob.GetLink())
				}
				if it.GetType() == vocab.FollowType {
					f := FollowRequest{}
					if err := f.FromActivityPub(a); err != nil {
						return err
					}
					follows = append(follows, f)
					relations.Store(a.GetLink(), a.GetLink())
				}
				if ValidModerationActivityTypes.Contains(typ) {
					m := ModerationOp{}
					if err := m.FromActivityPub(a); err != nil {
						return err
					}
					moderations = append(moderations, m)
					relations.Store(a.GetLink(), a.GetLink())
				}
				if ValidAppreciationTypes.Contains(typ) {
					v := Vote{}
					if err := v.FromActivityPub(a); err != nil {
						return err
					}
					appreciations = append(appreciations, v)
					relations.Store(a.GetLink(), a.GetLink())
				}
				return nil
			})
			if err == nil {
				for _, rem := range accumulateItemIRIs(it, deps) {
					if !deferredRemote.Contains(rem) {
						deferredRemote = append(deferredRemote, rem)
					}
				}
			}
		}
		// TODO(marius): this needs to be externalized also to a different function that we can pass from outer scope
		//   This function implements the logic for breaking out of the collection iteration cycle and returns a bool
		loadedCount := len(items) + len(follows) + len(accounts) + len(moderations) + len(appreciations)
		if f.MaxItems > 0 && loadedCount > 0 && loadedCount-f.MaxItems < 5 {
			return StopLoad{}
		}
		return nil
	})
	if err != nil {
		return emptyCursor, err
	}

	if len(deferredRemote) > 0 {
		for _, iri := range deferredRemote {
			ob, err := r.fedbox.client.LoadIRI(iri)
			if err != nil || vocab.IsNil(ob) {
				continue
			}
			typ := ob.GetType()
			if vocab.ActorTypes.Contains(typ) {
				ac := Account{}
				if err := ac.FromActivityPub(ob); err == nil {
					accounts = append(accounts, ac)
				}
			}
			if vocab.ObjectTypes.Contains(typ) {
				it := Item{}
				if err := it.FromActivityPub(ob); err == nil {
					items = append(items, it)
				}
			}
		}
	}

	if deps.Authors {
		items, _ = r.loadItemsAuthors(ctx, items...)
		accounts, _ = r.loadAccountsAuthors(ctx, accounts...)
	}
	if deps.Votes {
		items, _ = r.loadItemsVotes(ctx, items...)
	}
	if deps.Replies {
		if comments, err := r.loadItemsReplies(ctx, items...); err == nil {
			items = append(items, comments...)
		}
	}
	if deps.Follows {
		follows, _ = r.loadFollowsAuthors(ctx, follows...)
		for i, follow := range follows {
			for _, auth := range accounts {
				auth := auth
				fpub := follow.Object.AP()
				apub := auth.AP()
				if fpub != nil && apub != nil && fpub.GetLink().Equals(apub.GetLink(), false) {
					// NOTE(marius): this looks suspicious as fuck
					follows[i].Object = &auth
				}
			}
		}
	}
	moderations, _ = r.loadModerationDetails(ctx, moderations...)

	resM.Lock()
	defer resM.Unlock()
	relations.Range(func(_, value any) bool {
		rel, _ := value.(vocab.IRI)
		for i := range items {
			it := items[i]
			if it.Pub != nil && rel.Equals(it.AP().GetLink(), true) {
				result.Append(&it)
			}
		}
		for i := range follows {
			f := follows[i]
			if f.pub != nil && rel.Equals(f.AP().GetLink(), true) {
				result.Append(&f)
			}
		}
		for i := range accounts {
			a := accounts[i]
			if a.Pub != nil && rel.Equals(a.AP().GetLink(), true) {
				result.Append(&a)
			}
		}
		for i := range moderations {
			a := moderations[i]
			if rel.Equals(a.AP().GetLink(), true) {
				result.Append(&a)
			}
		}
		for i := range appreciations {
			a := appreciations[i]
			if a.Pub != nil && rel.Equals(a.AP().GetLink(), true) {
				result.Append(&a)
			}
		}
		return true
	})

	return Cursor{
		after:  HashFromString(next),
		before: HashFromString(prev),
		items:  result,
		total:  uint(len(result)),
	}, nil
}

func (r *repository) SaveVote(ctx context.Context, v Vote) (Vote, error) {
	if !v.SubmittedBy.IsValid() || !v.SubmittedBy.HasMetadata() {
		return Vote{}, errors.Newf("Invalid vote submitter")
	}
	if !v.Item.IsValid() || !v.Item.HasMetadata() {
		return Vote{}, errors.Newf("Invalid vote item")
	}
	author := r.loadAPPerson(*v.SubmittedBy)
	if !accountValidForC2S(v.SubmittedBy) {
		return v, errors.Unauthorizedf("invalid account %s", v.SubmittedBy.Handle)
	}

	item := *v.Item
	loadedItems, err := r.loadItemsVotes(ctx, item)
	item = loadedItems[0]
	// first step is to verify if vote already exists:
	if err != nil {
		r.errFn(log.Ctx{"err": err})("unable to load item votes")
	}
	var exists Vote
	for _, vot := range item.Votes {
		if !vot.SubmittedBy.IsValid() || !v.SubmittedBy.IsValid() {
			continue
		}
		if vot.SubmittedBy.Hash == v.SubmittedBy.Hash {
			exists = vot
			break
		}
	}

	o := new(vocab.Object)
	r.loadAPItem(o, *v.Item)
	act := &vocab.Activity{
		Type:  vocab.UndoType,
		Actor: author.GetLink(),
	}
	act.To, act.Bto, act.CC, act.BCC = r.defaultRecipientsList(v.SubmittedBy.Pub, Instance.Conf.PublicVotingEnabled)
	if Instance.Conf.PublicVotingEnabled {
		// NOTE(marius): if public voting is enabled we can append the recipients of the voted on object
		vocab.OnObject(act, func(ob *vocab.Object) error {
			return vocab.OnObject(v.Item.Pub, func(p *vocab.Object) error {
				appendRecipients(&ob.To, p.To)
				appendRecipients(&ob.Bto, p.Bto)
				appendRecipients(&ob.CC, p.CC)
				appendRecipients(&ob.BCC, p.BCC)
				return nil
			})
		})
	}

	if exists.HasMetadata() {
		act.Object = vocab.IRI(exists.Metadata.IRI)
		i, it, err := r.fedbox.ToOutbox(ctx, act)
		if err != nil {
			r.errFn()(err.Error())
		}
		r.cache.removeRelated(i, it, act)
	}

	if v.Weight > 0 && exists.Weight <= 0 {
		act.Type = vocab.LikeType
		act.Object = o.GetLink()
	} else if v.Weight < 0 && exists.Weight >= 0 {
		act.Type = vocab.DislikeType
		act.Object = o.GetLink()
	} else {
		return v, nil
	}
	if v.Item.SubmittedBy != nil && v.Item.SubmittedBy.Pub != nil {
		auth := v.Item.SubmittedBy.Pub
		if !auth.GetLink().Contains(r.BaseURL(), false) {
			// NOTE(marius): this assumes that the instance the user is from has a shared inbox at {instance_hostname}/inbox
			u, _ := auth.GetLink().URL()
			u.Path = ""
			act.BCC = append(act.BCC, vocab.IRI(u.String()))
		}
		act.To = append(act.To, auth.GetLink())
	}

	var (
		iri vocab.IRI
		it  vocab.Item
	)
	iri, it, err = r.fedbox.ToOutbox(ctx, act)
	lCtx := log.Ctx{"act": iri}
	if it != nil {
		lCtx["obj"] = it.GetLink()
		lCtx["type"] = it.GetType()
		r.cache.removeRelated(iri, it, act)
	}
	if err != nil && !errors.IsConflict(err) {
		r.errFn(lCtx)(err.Error())
		return v, err
	}
	err = v.FromActivityPub(act)
	r.infoFn()("saved activity")
	return v, err
}

type _errors struct {
	Ctxt   string        `jsonld:"@context"`
	Errors []errors.Http `jsonld:"errors"`
}

func (r *repository) handlerErrorResponse(body []byte) error {
	errs := _errors{}
	if err := j.Unmarshal(body, &errs); err != nil {
		r.errFn()("Unable to unmarshal error response: %s", err.Error())
		return nil
	}
	if len(errs.Errors) == 0 {
		return nil
	}
	err := errs.Errors[0]
	return errors.WrapWithStatus(err.Code, nil, err.Message)
}

func (r *repository) handleItemSaveSuccessResponse(ctx context.Context, it Item, body []byte) (Item, error) {
	ap, err := vocab.UnmarshalJSON(body)
	if err != nil {
		r.errFn()(err.Error())
		return it, err
	}
	if err = it.FromActivityPub(ap); err != nil {
		r.errFn()(err.Error())
		return it, err
	}
	items, err := r.loadItemsAuthors(ctx, it)
	return items[0], err
}

func accountValidForC2S(a *Account) bool {
	return a.IsValid() /*&& a.IsLogged()*/
}

func (r *repository) getAuthorRequestURL(a *Account) string {
	var reqURL string
	if a.IsValid() && a.IsLogged() {
		author := r.loadAPPerson(*a)
		if a.IsLocal() {
			reqURL = author.Outbox.GetLink().String()
		} else {
			reqURL = author.Inbox.GetLink().String()
		}
	} else {
		author := anonymousPerson(r.BaseURL())
		reqURL = author.Inbox.GetLink().String()
	}
	return reqURL
}

func loadCCsFromMentions(incoming []Tag) vocab.ItemCollection {
	if len(incoming) == 0 {
		return nil
	}

	iris := make(vocab.ItemCollection, 0)
	for _, inc := range incoming {
		if inc.Metadata != nil {
			iris = append(iris, vocab.IRI(inc.Metadata.ID))
		}
	}

	return iris
}

func loadMentionsIfExisting(r *repository, ctx context.Context, incoming TagCollection) TagCollection {
	if len(incoming) == 0 {
		return incoming
	}

	remoteFilters := make([]*Filters, 0)
	remoteWebfinger := make(map[string][]string, 0)
	for _, m := range incoming {
		// TODO(marius): we need to make a distinction between FedBOX remote servers and Webfinger remote servers
		u, err := url.ParseRequestURI(m.URL)
		if err != nil {
			continue
		}
		host := fmt.Sprintf("%s://%s", u.Scheme, u.Hostname())
		if strings.Contains(m.URL, "@"+m.Name) {
			// use webfinger
			remoteWebfinger[host] = append(remoteWebfinger[host], m.Name+"@"+u.Hostname())
			continue
		}
		if strings.Contains(r.SelfURL, host) {
			host = r.fedbox.baseURL.String()
		}
		urlFilter := EqualsString(host)
		var (
			filter *Filters
			add    bool
		)
		for i, fil := range remoteFilters {
			if fil.URL.Contains(urlFilter) {
				filter = remoteFilters[i]
			}
		}
		if filter == nil {
			filter = &Filters{Type: ActivityTypesFilter(vocab.PersonType)}
			filter.IRI = append(filter.IRI, urlFilter)
			add = true
		}
		nameFilter := EqualsString(m.Name)
		if !filter.Name.Contains(nameFilter) {
			filter.Name = append(filter.Name, nameFilter)
		}
		if add {
			remoteFilters = append(remoteFilters, filter)
		}
	}

	for _, filter := range remoteFilters {
		actorsIRI := actors.IRI(vocab.IRI(filter.IRI[0].Str))
		filter.IRI = nil
		col, err := r.fedbox.client.Collection(ctx, actorsIRI, Values(filter))
		if err != nil {
			r.errFn(log.Ctx{"err": err})("unable to load accounts from mentions")
			continue
		}
		vocab.OnCollectionIntf(col, func(col vocab.CollectionInterface) error {
			for _, it := range col.Collection() {
				for i, t := range incoming {
					vocab.OnActor(it, func(act *vocab.Actor) error {
						if strings.ToLower(t.Name) == strings.ToLower(string(act.Name.Get("-"))) ||
							strings.ToLower(t.Name) == strings.ToLower(string(act.PreferredUsername.Get("-"))) {
							url := act.ID.String()
							if act.URL != nil {
								url = act.URL.GetLink().String()
							}
							incoming[i].Metadata = &ItemMetadata{ID: act.ID.String(), URL: url}
							incoming[i].URL = url
						}
						return nil
					})
				}
			}
			return nil
		})
	}

	for h, accts := range remoteWebfinger {
		for _, acct := range accts {
			act, err := r.loadWebfingerActorFromIRI(context.TODO(), h, acct)
			if err != nil {
				r.errFn(log.Ctx{"err": err, "host": h, "account": acct})("unable to load account")
				continue
			}

			for i, t := range incoming {
				if strings.ToLower(t.Name) == strings.ToLower(string(act.Name.Get("-"))) ||
					strings.ToLower(t.Name) == strings.ToLower(string(act.PreferredUsername.Get("-"))) {
					url := act.ID.String()
					if act.URL != nil {
						url = act.URL.GetLink().String()
					}
					incoming[i].Metadata = &ItemMetadata{ID: act.ID.String(), URL: url}
					incoming[i].URL = url
				}
			}
		}
	}

	return incoming
}

func loadTagsIfExisting(r *repository, ctx context.Context, incoming TagCollection) TagCollection {
	if len(incoming) == 0 {
		return incoming
	}

	tagNames := make(CompStrs, 0)
	for _, t := range incoming {
		tagNames = append(tagNames, EqualsString(t.Name), EqualsString("#"+t.Name))
	}
	ff := &Filters{Name: tagNames}
	tags, _, err := r.LoadTags(ctx, ff)
	if err != nil {
		r.errFn(log.Ctx{"err": err})("unable to load accounts from mentions")
	}

	for i, t := range incoming {
		for _, tag := range tags {
			if tag.Metadata == nil || len(tag.Metadata.ID) == 0 {
				continue
			}
			if strings.ToLower(t.Name) == strings.ToLower(tag.Name) ||
				strings.ToLower("#"+t.Name) == strings.ToLower(tag.Name) {
				incoming[i] = tag
			}
		}
	}
	return incoming
}

func (r *repository) loadFromAccountForDelete(ctx context.Context, art *vocab.Actor, it *Account) error {
	if it.HasMetadata() {
		m := it.Metadata
		m.Tags = loadTagsIfExisting(r, ctx, m.Tags)
		it.Metadata = m
	}
	par := it.Parent
	art.To, art.Bto, art.CC, art.BCC = r.defaultRecipientsList(nil, !it.Private())
	for {
		// Appending parents' authors to the CC of current activity
		if par == nil {
			break
		}
		p, ok := par.(*Item)
		if !ok {
			continue
		}
		if parAuth := p.SubmittedBy; parAuth != nil {
			art.CC.Append(parAuth.Pub.GetLink())
		}
		par = p.Parent
	}

	*art = *r.loadAPPerson(*it)
	if it.Parent != nil {
		vocab.OnObject(it.Parent.AP(), func(p *vocab.Object) error {
			appendRecipients(&art.To, p.To)
			appendRecipients(&art.Bto, p.Bto)
			appendRecipients(&art.CC, p.CC)
			appendRecipients(&art.BCC, p.BCC)
			return nil
		})
	}

	return nil
}

func (r *repository) loadFromItemForDelete(ctx context.Context, art *vocab.Object, it *Item) error {
	it.Delete()

	if it.HasMetadata() {
		m := it.Metadata
		if len(m.To) > 0 {
			for _, rec := range m.To {
				if rr := vocab.IRI(rec.Metadata.ID); !art.CC.Contains(rr) {
					art.To = append(art.To, rr)
				}
			}
		}
		if len(m.CC) > 0 {
			for _, rec := range m.CC {
				if rr := vocab.IRI(rec.Metadata.ID); !art.CC.Contains(rr) {
					art.CC = append(art.CC, rr)
				}
			}
		}
		m.Tags = loadTagsIfExisting(r, ctx, m.Tags)
		m.Mentions = loadMentionsIfExisting(r, ctx, m.Mentions)
		for _, mm := range loadCCsFromMentions(m.Mentions) {
			if !art.CC.Contains(mm) {
				art.CC = append(art.CC, mm)
			}
		}
		it.Metadata = m
	}
	par := it.Parent
	for {
		// Appending parents' authors to the CC of current activity
		if par == nil {
			break
		}
		p, ok := par.(*Item)
		if !ok {
			continue
		}
		if parAuth := p.SubmittedBy; parAuth != nil && !art.CC.Contains(parAuth.Pub.GetLink()) {
			art.CC = append(art.CC, parAuth.Pub.GetLink())
		}
		par = p.Parent
	}

	if !it.Private() {
		if it.Parent == nil && it.SubmittedBy.HasMetadata() && len(it.SubmittedBy.Metadata.FollowersIRI) > 0 {
			art.CC = append(art.CC, vocab.IRI(it.SubmittedBy.Metadata.FollowersIRI))
		}
	}

	r.loadAPItem(art, *it)
	if it.Parent != nil {
		return loadFromParent(art, it.Parent.AP())
	}

	return nil
}

func (r *repository) ModerateDelete(ctx context.Context, mod ModerationOp, author *Account) (ModerationOp, error) {
	var (
		err      error
		toDelete vocab.Item
		actType  = vocab.DeleteType
	)
	if len(mod.Object.ID()) == 0 {
		r.errFn(log.Ctx{"item": mod.Object})(err.Error())
		return mod, errors.NotFoundf("item hash is empty, can not delete")
	}
	switch p := mod.Object.(type) {
	case *Item:
		ob := new(vocab.Object)
		err = r.loadFromItemForDelete(ctx, ob, p)
		toDelete = ob
	case *Account:
		act := new(vocab.Actor)
		err = r.loadFromAccountForDelete(ctx, act, p)
		toDelete = act
	default:
		return ModerationOp{}, errors.Newf("invalid moderation type object")
	}

	act := &vocab.Activity{
		AttributedTo: author.AP(),
		Actor:        r.app.AP().GetLink(),
		Type:         actType,
		Object:       toDelete.GetID(),
	}
	vocab.OnObject(toDelete, func(ob *vocab.Object) error {
		act.To = ob.To
		act.Bto = ob.Bto
		act.CC = ob.CC
		act.BCC = ob.BCC
		return nil
	})

	i, tombstone, err := r.fedbox.ToOutbox(ctx, act)
	if err != nil && !errors.IsGone(err) {
		r.errFn()(err.Error())
		return mod, err
	}
	lCtx := log.Ctx{"iri": i}
	if !vocab.IsNil(tombstone) {
		lCtx["tombstone"] = tombstone.GetLink()
		lCtx["type"] = tombstone.GetType()
	}
	r.infoFn(lCtx)("saved activity")
	r.cache.removeRelated(act, tombstone)
	return mod, err
}

func (r *repository) defaultRecipientsList(act vocab.Item, withPublic bool) (vocab.ItemCollection, vocab.ItemCollection, vocab.ItemCollection, vocab.ItemCollection) {
	var bto vocab.ItemCollection = nil

	to := make(vocab.ItemCollection, 0)
	cc := make(vocab.ItemCollection, 0)
	bcc := make(vocab.ItemCollection, 0)

	if withPublic {
		to.Append(vocab.PublicNS)
		if act != nil {
			cc.Append(vocab.Followers.IRI(act))
		}
		bcc.Append(r.fedbox.Service().ID)
	}
	// NOTE(marius): we publish the activity just to the instance and its followers
	cc.Append(r.app.Pub.GetLink(), vocab.Followers.IRI(r.app.Pub))

	return to, bto, cc, bcc
}

func wrapItemInCreate(it vocab.Item, author vocab.Item) *vocab.Activity {
	act := &vocab.Activity{
		Type:   vocab.CreateType,
		Actor:  author.GetLink(),
		Object: it,
	}
	return act
}

func (r *repository) SaveItem(ctx context.Context, it Item) (Item, error) {
	if it.SubmittedBy == nil || !it.SubmittedBy.HasMetadata() {
		return Item{}, errors.Newf("invalid account")
	}
	var author *vocab.Actor
	if it.SubmittedBy.IsLogged() {
		author = r.loadAPPerson(*it.SubmittedBy)
	} else {
		author = anonymousPerson(r.BaseURL())
	}
	if !accountValidForC2S(it.SubmittedBy) {
		return it, errors.Unauthorizedf("invalid account %s", it.SubmittedBy.Handle)
	}

	to, _, cc, bcc := r.defaultRecipientsList(author, it.Public())

	var err error

	if it.HasMetadata() {
		m := it.Metadata
		if len(m.To) > 0 {
			for _, rec := range m.To {
				if rr := vocab.IRI(rec.Metadata.ID); !cc.Contains(rr) {
					to = append(to, rr)
				}
			}
		}
		if len(m.CC) > 0 {
			for _, rec := range m.CC {
				if rr := vocab.IRI(rec.Metadata.ID); !cc.Contains(rr) {
					cc = append(cc, rr)
				}
			}
		}
		m.Tags = loadTagsIfExisting(r, ctx, m.Tags)
		m.Mentions = loadMentionsIfExisting(r, ctx, m.Mentions)
		for _, mm := range loadCCsFromMentions(m.Mentions) {
			if !cc.Contains(mm) {
				cc = append(cc, mm)
			}
		}
		it.Metadata = m
	}
	par := it.Parent
	for {
		// Appending parents' authors to the CC of current activity
		if par == nil {
			break
		}
		p, ok := par.(*Item)
		if !ok {
			continue
		}
		if parAuth := p.SubmittedBy; parAuth != nil && !cc.Contains(parAuth.Pub.GetLink()) {
			cc = append(cc, parAuth.Pub.GetLink())
		}
		par = p.Parent
	}

	if !it.Private() {
		if it.Parent == nil && it.SubmittedBy.HasMetadata() && len(it.SubmittedBy.Metadata.FollowersIRI) > 0 {
			cc = append(cc, vocab.IRI(it.SubmittedBy.Metadata.FollowersIRI))
		}
	}

	art := new(vocab.Object)
	r.loadAPItem(art, it)
	if it.Parent != nil {
		loadFromParent(art, it.Parent.AP())
	}
	id := art.GetLink()

	act := &vocab.Activity{
		To:     to,
		CC:     cc,
		BCC:    bcc,
		Actor:  author.GetLink(),
		Object: art,
	}
	loadAuthors := true
	if it.Deleted() {
		if len(id) == 0 {
			r.errFn(log.Ctx{"item": it.Hash})(err.Error())
			return it, errors.NotFoundf("item hash is empty, can not delete")
		}
		act.Object = id
		act.Type = vocab.DeleteType
		loadAuthors = false
	} else {
		if len(id) == 0 {
			act.Type = vocab.CreateType
		} else {
			act.Type = vocab.UpdateType
		}
	}
	var (
		i  vocab.IRI
		ob vocab.Item
	)
	i, ob, err = r.fedbox.ToOutbox(ctx, act)
	lCtx := log.Ctx{"act": i}
	if !vocab.IsNil(ob) {
		lCtx["obj"] = ob.GetLink()
		lCtx["type"] = ob.GetType()
	}
	if err != nil && !(it.Deleted() && errors.IsGone(err)) {
		r.errFn(lCtx)(err.Error())
		return it, err
	}
	r.cache.removeRelated(i, ob, act)
	r.infoFn(lCtx)("saved activity")
	if err = it.FromActivityPub(ob); err != nil {
		r.errFn(lCtx)(err.Error())
		return it, err
	}
	if loadAuthors {
		items, err := r.loadItemsAuthors(ctx, it)
		return items[0], err
	}
	return it, err
}

func (r *repository) LoadTags(ctx context.Context, ff ...*Filters) (TagCollection, uint, error) {
	tags := make(TagCollection, 0)
	var count uint = 0

	fns := make([]func() error, 0)
	for _, f := range ff {
		fns = append(fns, func() error {
			it, err := r.fedbox.Objects(ctx, Values(f))
			if err != nil {
				r.errFn()(err.Error())
				return err
			}
			return vocab.OnOrderedCollection(it, func(col *vocab.OrderedCollection) error {
				count = col.TotalItems
				for _, it := range col.OrderedItems {
					tag := Tag{}
					if err := tag.FromActivityPub(it); err != nil {
						r.errFn(log.Ctx{"type": fmt.Sprintf("%T", it)})(err.Error())
						continue
					}
					tags = append(tags, tag)
				}
				return nil
			})
		})
	}
	if err := flowmatic.Do(fns...); err != nil {
		return tags, count, err
	}
	return tags, count, nil
}

type CollectionFilterFn func(context.Context, ...client.FilterFn) (vocab.CollectionInterface, error)

func (r *repository) LoadAccounts(ctx context.Context, colFn CollectionFilterFn, ff ...*Filters) (AccountCollection, uint, error) {
	accounts := make(AccountCollection, 0)
	var count uint = 0

	if colFn == nil {
		colFn = r.fedbox.Actors
	}

	fns := make([]func() error, 0)
	for _, f := range ff {
		fns = append(fns, func() error {
			it, err := colFn(ctx, Values(f))
			if err != nil {
				r.errFn()(err.Error())
				return err
			}
			return vocab.OnOrderedCollection(it, func(col *vocab.OrderedCollection) error {
				count = col.TotalItems
				for _, it := range col.OrderedItems {
					acc := Account{Metadata: &AccountMetadata{}}
					if err := acc.FromActivityPub(it); err != nil {
						r.errFn(log.Ctx{"type": fmt.Sprintf("%T", it)})(err.Error())
						continue
					}
					accounts = append(accounts, acc)
				}
				return err
			})
		})
	}

	if err := flowmatic.Do(fns...); err != nil {
		return accounts, count, err
	}
	return accounts, count, nil
}

func (r *repository) LoadAccountDetails(ctx context.Context, acc *Account) error {
	r.WithAccount(acc)
	now := time.Now().UTC()
	lastUpdated := acc.Metadata.OutboxUpdated
	if now.Sub(lastUpdated)-5*time.Minute < 0 {
		return nil
	}
	var err error
	ltx := log.Ctx{"handle": acc.Handle, "hash": acc.Hash}
	r.infoFn(ltx)("loading account details")

	if err = r.loadAccountsOutbox(ctx, acc); err != nil {
		r.infoFn(ltx, log.Ctx{"err": err.Error()})("unable to load outbox")
	}
	if len(acc.Followers) == 0 {
		// TODO(marius): this needs to be moved to where we're handling all Inbox activities, not on page load
		if err = r.loadAccountsFollowers(ctx, acc); err != nil {
			r.infoFn(ltx, log.Ctx{"err": err.Error()})("unable to load followers")
		}
	}
	if len(acc.Following) == 0 {
		if err = r.loadAccountsFollowing(ctx, acc); err != nil {
			r.infoFn(ltx, log.Ctx{"err": err.Error()})("unable to load following")
		}
	}
	acc.Metadata.OutboxUpdated = now
	return nil
}

func (r *repository) LoadAccount(ctx context.Context, iri vocab.IRI) (*Account, error) {
	acc := new(Account)
	act, err := r.fedbox.Actor(ctx, iri)
	if err != nil {
		r.errFn()(err.Error())
		return acc, err
	}
	err = acc.FromActivityPub(act)
	if err != nil {
		return acc, err
	}
	err = r.LoadAccountDetails(ctx, acc)
	return acc, err
}

func Values(f interface{}) client.FilterFn {
	return func() url.Values {
		v, e := qstring.Marshal(f)
		if e != nil {
			return url.Values{}
		}
		return v
	}
}

func (r *repository) LoadFollowRequests(ctx context.Context, ed *Account, f *Filters) (FollowRequests, uint, error) {
	if len(f.Type) == 0 {
		f.Type = ActivityTypesFilter(vocab.FollowType)
		f.Actor = derefIRIFilters
	}
	var followReq vocab.CollectionInterface
	var err error
	if ed == nil {
		followReq, err = r.fedbox.Activities(ctx, Values(f))
	} else {
		followReq, err = r.fedbox.Inbox(ctx, r.loadAPPerson(*ed), Values(f))
	}
	requests := make([]FollowRequest, 0)
	if err == nil && len(followReq.Collection()) > 0 {
		for _, fr := range followReq.Collection() {
			f := new(FollowRequest)
			if err := f.FromActivityPub(fr); err == nil {
				if !accountInCollection(*f.SubmittedBy, ed.Followers) {
					requests = append(requests, *f)
				}
			}
		}
		requests, err = r.loadFollowsAuthors(ctx, requests...)
	}
	return requests, uint(len(requests)), nil
}

func (r *repository) SendFollowResponse(ctx context.Context, f FollowRequest, accept bool, reason *Item) error {
	er := f.SubmittedBy
	if !er.IsValid() {
		return errors.Newf("invalid account that wants to follow %s", er.Handle)
	}
	ed := f.Object
	if !accountValidForC2S(ed) {
		return errors.Unauthorizedf("invalid account for request %s", ed.Handle)
	}

	response := new(vocab.Activity)
	response.To, _, response.CC, response.BCC = r.defaultRecipientsList(er.Pub, accept)
	appendRecipients(&response.To, vocab.IRI(er.Metadata.ID))
	response.Type = vocab.RejectType
	if accept {
		response.Type = vocab.AcceptType
	}
	if reason != nil {
		r.loadAPItem(response, *reason)
	}
	response.Object = vocab.IRI(f.Metadata.ID)
	response.Actor = vocab.IRI(ed.Metadata.ID)

	i, it, err := r.fedbox.ToOutbox(ctx, response)
	if err != nil && !errors.IsConflict(err) {
		r.errFn(log.Ctx{
			"err":      err,
			"follower": er.Handle,
			"followed": ed.Handle,
		})("unable to respond to follow")
		return err
	}
	r.cache.removeRelated(i, it, response)
	return nil
}

func (r *repository) FollowAccount(ctx context.Context, er, ed Account, reason *Item) error {
	if !accountValidForC2S(&er) {
		return errors.Unauthorizedf("invalid account %s", er.Handle)
	}
	follower := r.loadAPPerson(er)
	followed := r.loadAPPerson(ed)
	err := r.FollowActor(ctx, follower, followed, reason)
	r.errFn(log.Ctx{
		"err":      err,
		"follower": er.Handle,
		"followed": ed.Handle,
	})("Unable to follow")
	return err
}

func (r *repository) FollowActor(ctx context.Context, follower, followed *vocab.Actor, reason *Item) error {
	follow := new(vocab.Follow)
	follow.To, _, follow.CC, follow.BCC = r.defaultRecipientsList(follower, true)
	appendRecipients(&follow.To, followed.GetLink())
	if reason != nil {
		r.loadAPItem(follow, *reason)
	}
	follow.Type = vocab.FollowType
	follow.Object = followed.GetLink()
	follow.Actor = follower.GetLink()
	_, _, err := r.fedbox.ToOutbox(ctx, follow)
	if err != nil {
		return err
	}
	return nil
}

func (r *repository) SaveAccount(ctx context.Context, a Account) (Account, error) {
	p := r.loadAPPerson(a)
	id := p.GetLink()

	now := time.Now().UTC()

	if p.Published.IsZero() {
		p.Published = now
	}
	p.Updated = now

	fx := r.fedbox.Service()
	parent := fx
	if a.CreatedBy != nil {
		parent = r.loadAPPerson(*a.CreatedBy)
	}

	act := vocab.Activity{Updated: now}
	act.To, _, act.CC, act.BCC = r.defaultRecipientsList(parent, true)

	act.AttributedTo = parent.GetLink()
	act.Actor = parent.GetLink()
	var err error
	if a.Deleted() {
		if len(id) == 0 {
			err := errors.NotFoundf("item hash is empty, can not delete")
			r.infoFn(log.Ctx{
				"actor":  a.GetLink(),
				"parent": parent.GetLink(),
				"err":    err,
			})("account save failed")
			return a, err
		}
		act.Type = vocab.DeleteType
		act.Object = id
	} else {
		act.Object = p
		p.To = act.To
		p.BCC = act.BCC
		if len(id) == 0 {
			act.Type = vocab.CreateType
		} else {
			act.Type = vocab.UpdateType
		}
	}

	var ap vocab.Item
	ltx := log.Ctx{"actor": a.Handle}
	i, ap, err := r.fedbox.ToOutbox(ctx, act)
	if err != nil {
		ltx["parent"] = parent.GetLink()
		if ap != nil {
			ltx["activity"] = ap.GetLink()
		}
		r.errFn(ltx, log.Ctx{"err": err})("account save failed")
		return a, err
	}
	r.cache.removeRelated(i, ap, act)
	if err := a.FromActivityPub(ap); err != nil {
		r.errFn(ltx, log.Ctx{"err": err})("loading of actor from JSON failed")
	}
	return a, nil
}

// LoadInfo this method is here to keep compatibility with the repository interfaces
// but in the long term we might want to store some of this information in the DB
func (r *repository) LoadInfo() (WebInfo, error) {
	return Instance.NodeInfo(), nil
}

func (r repository) moderationActivity(ctx context.Context, er *vocab.Actor, ed vocab.Item, reason *Item) (*vocab.Activity, error) {
	act := new(vocab.Activity)
	act.To, _, act.CC, act.BCC = r.defaultRecipientsList(er, false)

	// We need to add the ed/er accounts' creators to the CC list
	if er.AttributedTo != nil && !er.AttributedTo.GetLink().Equals(vocab.PublicNS, true) {
		appendRecipients(&act.CC, er.AttributedTo.GetLink())
	}
	vocab.OnObject(ed, func(o *vocab.Object) error {
		if o.AttributedTo != nil {
			auth, err := r.fedbox.Actor(ctx, o.AttributedTo.GetLink())
			if err == nil && auth != nil && auth.AttributedTo != nil &&
				!(auth.AttributedTo.GetLink().Equals(auth.GetLink(), false) || auth.AttributedTo.GetLink().Equals(vocab.PublicNS, true)) {
				appendRecipients(&act.CC, auth.AttributedTo.GetLink())
			}
		}
		return nil
	})
	if reason != nil {
		reason.MakePrivate()
		r.loadAPItem(act, *reason)
	}
	act.Object = ed.GetLink()
	act.Actor = er.GetLink()
	return act, nil
}

func (r repository) moderationActivityOnItem(ctx context.Context, er Account, ed Item, reason *Item) (*vocab.Activity, error) {
	reporter := r.loadAPPerson(er)
	reported := new(vocab.Object)
	r.loadAPItem(reported, ed)
	if !accountValidForC2S(&er) {
		return nil, errors.Unauthorizedf("invalid account %s", er.Handle)
	}
	return r.moderationActivity(ctx, reporter, reported, reason)
}

func (r repository) moderationActivityOnAccount(ctx context.Context, er, ed Account, reason *Item) (*vocab.Activity, error) {
	reporter := r.loadAPPerson(er)
	reported := r.loadAPPerson(ed)
	if !accountValidForC2S(&er) {
		return nil, errors.Unauthorizedf("invalid account %s", er.Handle)
	}

	return r.moderationActivity(ctx, reporter, reported, reason)
}

func (r *repository) BlockAccount(ctx context.Context, er, ed Account, reason *Item) error {
	block, err := r.moderationActivityOnAccount(ctx, er, ed, reason)
	if err != nil {
		r.errFn()(err.Error())
		return err
	}
	block.Type = vocab.BlockType
	i, ob, err := r.fedbox.ToOutbox(ctx, block)

	lCtx := log.Ctx{"activity": i}
	if !vocab.IsNil(ob) {
		lCtx["object"] = ob.GetLink()
		lCtx["type"] = ob.GetType()
	}
	if err != nil {
		r.errFn(lCtx)(err.Error())
		return err
	}
	r.cache.removeRelated(i, ob)
	return nil
}

func (r *repository) BlockItem(ctx context.Context, er Account, ed Item, reason *Item) error {
	block, err := r.moderationActivityOnItem(ctx, er, ed, reason)
	if err != nil {
		r.errFn()(err.Error())
		return err
	}
	block.Type = vocab.BlockType
	i, ob, err := r.fedbox.ToOutbox(ctx, block)
	lCtx := log.Ctx{"activity": i}
	if !vocab.IsNil(ob) {
		lCtx["object"] = ob.GetLink()
		lCtx["type"] = ob.GetType()
	}
	if err != nil {
		r.errFn(lCtx)(err.Error())
		return err
	}
	r.cache.removeRelated(i, ob, block)
	return nil
}

func (r *repository) ReportItem(ctx context.Context, er Account, it Item, reason *Item) error {
	flag, err := r.moderationActivityOnItem(ctx, er, it, reason)
	if err != nil {
		r.errFn()(err.Error())
		return err
	}
	flag.Type = vocab.FlagType
	i, ob, err := r.fedbox.ToOutbox(ctx, flag)
	if err != nil {
		r.errFn()(err.Error())
		return err
	}
	r.cache.removeRelated(i, ob, flag)
	return nil
}

func (r *repository) ReportAccount(ctx context.Context, er, ed Account, reason *Item) error {
	flag, err := r.moderationActivityOnAccount(ctx, er, ed, reason)
	if err != nil {
		r.errFn()(err.Error())
		return err
	}
	flag.Type = vocab.FlagType
	i, ob, err := r.fedbox.ToOutbox(ctx, flag)
	if err != nil {
		r.errFn()(err.Error())
		return err
	}
	r.cache.removeRelated(i, ob, flag)
	return nil
}

type StopLoad struct{}

func (s StopLoad) Error() string {
	return "this is the end"
}

type RemoteLoads map[vocab.IRI][]RemoteLoad

type RemoteLoad struct {
	actor   vocab.Item
	signFn  client.RequestSignFn
	loadFn  LoadFn
	filters []*Filters
}

type StopSearchErr string

func (s StopSearchErr) Error() string {
	return string(s)
}

func (r *repository) loadItemFromCacheOrIRI(ctx context.Context, iri vocab.IRI) (vocab.Item, error) {
	if it, okCache := r.cache.get(iri); okCache {
		if getItemUpdatedTime(it).Sub(time.Now()) < 10*time.Minute {
			return it, nil
		}
	}
	return r.fedbox.client.CtxLoadIRI(ctx, iri)
}

func (r *repository) loadCollectionFromCacheOrIRI(ctx context.Context, iri vocab.IRI) (vocab.CollectionInterface, bool, error) {
	if it, okCache := r.cache.get(cacheKey(iri, ContextAccount(ctx))); okCache {
		if c, okCol := it.(vocab.CollectionInterface); okCol && getItemUpdatedTime(it).Sub(time.Now()) < 10*time.Minute && c.Count() > 0 {
			return c, true, nil
		}
	}
	col, err := r.fedbox.collection(ctx, iri)
	return col, false, err
}

type searchFn func(ctx context.Context, col vocab.CollectionInterface, f *Filters) error

func (r *repository) searchFn(ctx context.Context, curIRI vocab.IRI, f *Filters, fn searchFn) func() error {
	return func() error {
		loadIRI := iri(curIRI, Values(f))

		col, fromCache, err := r.loadCollectionFromCacheOrIRI(ctx, loadIRI)
		if err != nil {
			return errors.Annotatef(err, "failed to load search: %s", loadIRI)
		}

		maxItems := 0
		err = vocab.OnCollectionIntf(col, func(c vocab.CollectionInterface) error {
			maxItems = int(c.Count())
			return fn(ctx, c, f)
		})
		if !fromCache {
			r.cache.add(cacheKey(loadIRI, ContextAccount(ctx)), col)
		}
		if err != nil {
			return err
		}

		if maxItems-f.MaxItems < 5 {
			if _, f.Next = getCollectionPrevNext(col); len(f.Next) > 0 {
				if err = flowmatic.Do(r.searchFn(ctx, curIRI, f, fn)); err != nil {
					return err
				}
			}
		} else {
			return StopLoad{}
		}

		return nil
	}
}

func cacheKey(i vocab.IRI, a *Account) vocab.IRI {
	if a == nil || !a.IsLogged() {
		return i
	}
	u, err := i.URL()
	if err != nil {
		return i
	}
	u.User = url.User(a.Hash.String())
	return vocab.IRI(u.String())
}

func LoadFromSearches(ctx context.Context, repo *repository, loads RemoteLoads, fn searchFn) error {
	var cancelFn func()

	ctx, cancelFn = context.WithCancel(ctx)

	var err error
	for service, searches := range loads {
		for _, search := range searches {
			for _, f := range search.filters {
				if search.loadFn == nil {
					continue
				}
				if search.actor == nil {
					search.actor = service
				}
				if search.signFn != nil {
					// NOTE(marius): this should be added in a cleaner way
					repo.fedbox.client.SignFn(search.signFn)
				}
				err = flowmatic.Do(repo.searchFn(ctx, search.loadFn(search.actor), f, fn))
			}
		}
	}
	if err != nil {
		if xerrors.Is(err, StopLoad{}) {
			repo.infoFn()("stopped loading search")
			cancelFn()
		} else {
			repo.errFn(log.Ctx{"err": err.Error()})("Failed to load search")
		}
	}
	return nil
}
