package brutalinks

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"git.sr.ht/~mariusor/box"
	log "git.sr.ht/~mariusor/lw"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/client/credentials"
	"github.com/go-ap/errors"
	"github.com/go-ap/filters"
	j "github.com/go-ap/jsonld"
	"github.com/mariusor/qstring"
)

type repository struct {
	SelfURL string
	b       *box.Client
	cred    *credentials.C2S
	cache   *cc
	app     *Account
	fedbox  *fedbox
	modTags TagCollection
	infoFn  CtxLogFn
	errFn   CtxLogFn
}

func (r *repository) BaseURL() vocab.IRI {
	return r.fedbox.conf.BaseURL
}

func (r *repository) Close() error {
	return r.b.Close()
}

func IsNotExist(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist)
}

func LoadCredentials(b *box.Client, c appConfig) (*credentials.C2S, error) {
	cred, err := box.LoadCredentials(b, vocab.IRI(c.OAuth2App))
	if err == nil {
		return cred, err
	}
	auth := credentials.ClientConfig{
		ClientID:     c.OAuth2App,
		ClientSecret: c.OAuth2Secret,
		RedirectURL:  fmt.Sprintf("%s/auth/%s/callback", c.BaseURL, "fedbox"),
	}
	if cred, err = credentials.Authorize(context.Background(), c.OAuth2App, auth); err != nil {
		return nil, err
	}
	err = box.SaveCredentials(b, *cred)
	return cred, err
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

	repo := &repository{
		SelfURL: c.BaseURL,
		infoFn:  infoFn,
		errFn:   errFn,
		cache:   caches(c.CachingEnabled),
	}

	storeFn := box.UseXDGPaths(c.HostName)
	if c.StoragePath != "" {
		storeFn = box.UseBasePath(c.StoragePath)
	}
	ua := fmt.Sprintf("%s (+https://github.com/mariusor/brutalinks@%s)", c.HostName, c.Version)

	var err error
	repo.b, err = box.New(storeFn, box.UseLogger(c.Logger.WithContext(log.Ctx{"log": "box"})), box.WithUserAgent(ua))
	if err != nil {
		return repo, err
	}
	if err = repo.b.Open(); err != nil {
		return repo, err
	}
	c.Logger.WithContext(log.Ctx{"path": repo.b.StoragePath()}).Infof("BOX storage opened")

	if c.OAuth2App == "" {
		return repo, fmt.Errorf("invalid OAuth2 application name %s", c.OAuth2App)
	}

	cred, err := LoadCredentials(repo.b, c)
	if err != nil {
		return repo, fmt.Errorf("unable to load credentials or authorize Actor: %w", err)
	}

	repo.fedbox, err = NewClient(
		WithURL(c.APIURL),
		WithUserAgent(ua),
		WithLogger(c.Logger),
		WithOAuth2(cred),
		SkipTLSCheck(!c.Env.IsProd()),
	)
	if err != nil {
		return repo, err
	}

	if actor := box.Author(repo.b, cred); actor != nil {
		repo.app = new(Account)
		if err := repo.app.FromActivityPub(actor); err != nil {
			l.WithContext(log.Ctx{"err": err.Error()}).Errorf("unable to load instance Actor")
		}

		repo.cred = cred
	} else {
		return repo, fmt.Errorf("invalid authorized Actor: %s", cred.IRI)
	}

	go func() {
		// NOTE(marius): this is the new BrutaLinks long polling mechanism that fetches
		// the relevant collections for the instance actor every minute.
		ctx := context.TODO()
		if err := repo.b.Follow(ctx); err != nil {
			c.Logger.WithContext(log.Ctx{"err": err.Error()}).Warnf("error fetching remotes")
		}
	}()

	if repo.modTags, err = SaveModeratorTags(repo); err != nil {
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

	for i, tag := range toSaveTags {
		ap := buildAPTagObject(tag, repo)
		create := wrapItemInCreate(ap, repo.app.Pub)
		create.To, _, create.CC, create.BCC = repo.defaultRecipientsList(repo.app.Pub, true)
		_, tagIt, err := repo.ToOutbox(context.TODO(), *repo.cred, create)
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
	t.Name = make(vocab.NaturalLanguageValues)
	t.Summary = make(vocab.NaturalLanguageValues)
	t.AttributedTo = repo.app.Pub.GetLink()
	_ = t.Name.Set(vocab.NilLangRef, vocab.Content(tag.Name))
	_ = t.Summary.Set(vocab.NilLangRef, vocab.Content(fmt.Sprintf("Moderator tag for instance %s", repo.SelfURL)))
	t.URL = vocab.IRI(tag.URL)
	return t
}

func LoadModeratorTags(repo *repository) (TagCollection, error) {
	ff := filters.All(
		filters.Any(filters.NameIs(tagNameModerator), filters.NameIs(tagNameSysOP)),
		//Type:   nilFilters, // TODO(marius): this seems to have a problem currently on FedBOX
		filters.SameAttributedTo(repo.app.AP().GetID()),
	)
	tags, _, err := repo.LoadTags(context.Background(), ff)
	return tags, err
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
	_ = vocab.OnObject(it, func(p *vocab.Object) error {
		_ = appendRecipients(&ob.To, p.To)
		_ = appendRecipients(&ob.Bto, p.Bto)
		_ = appendRecipients(&ob.CC, p.CC)
		_ = appendRecipients(&ob.BCC, p.BCC)
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

			switch item.MimeType {
			case MimeTypeMarkdown:
				o.Source.MediaType = vocab.MimeType(item.MimeType)
				o.MediaType = MimeTypeHTML
				if item.Data != "" {
					if o.Source.Content == nil {
						o.Source.Content = make(vocab.NaturalLanguageValues)
					}
					_ = o.Source.Content.Set(vocab.DefaultLang, vocab.Content(item.Data))
					if o.Content == nil {
						o.Content = make(vocab.NaturalLanguageValues)
					}
					_ = o.Content.Set(vocab.DefaultLang, vocab.Content(Markdown(item.Data)))
				}
			case MimeTypeText:
				fallthrough
			case MimeTypeHTML:
				o.MediaType = vocab.MimeType(item.MimeType)
				if o.Content == nil {
					o.Content = make(vocab.NaturalLanguageValues)
				}
				_ = o.Content.Set(vocab.DefaultLang, vocab.Content(item.Data))
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
			if o.Name == nil {
				o.Name = make(vocab.NaturalLanguageValues)
			}
			_ = o.Name.Set(vocab.DefaultLang, vocab.Content(item.Title))
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
								_ = appendRecipients(&to, pAuth)
							}
							first = false
						} else if !cc.Contains(pAuth) {
							_ = appendRecipients(&cc, pAuth)
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
					_ = appendRecipients(&to, mto)
				}
			}
			for _, rec := range m.CC {
				mcc := vocab.IRI(rec.Metadata.ID)
				if !cc.Contains(mcc) {
					_ = appendRecipients(&cc, mcc)
				}
			}
			if m.Mentions != nil || m.Tags != nil {
				o.Tag = make(vocab.ItemCollection, 0)
				for _, men := range m.Mentions {
					// todo(marius): retrieve object ids of each mention and add it to the CC of the object
					t := vocab.Mention{
						Type: vocab.MentionType,
						Name: vocab.NaturalLanguageValues{vocab.NilLangRef: vocab.Content(men.Name)},
						Href: vocab.IRI(men.URL),
					}
					if men.Metadata != nil && len(men.Metadata.ID) > 0 {
						t.ID = vocab.IRI(men.Metadata.ID)
					}
					_ = o.Tag.Append(t)
				}
				for _, tag := range m.Tags {
					name := "#" + tag.Name
					if tag.Name[0] == '#' {
						name = tag.Name
					}
					t := vocab.Object{
						URL:  vocab.ID(tag.URL),
						Name: vocab.NaturalLanguageValues{vocab.NilLangRef: vocab.Content(name)},
					}
					if tag.Metadata != nil && len(tag.Metadata.ID) > 0 {
						t.ID = vocab.IRI(tag.Metadata.ID)
					}
					_ = o.Tag.Append(t)
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
	Name:              vocab.NaturalLanguageValues{vocab.NilLangRef: vocab.Content(Anonymous)},
	Type:              vocab.PersonType,
	PreferredUsername: vocab.NaturalLanguageValues{vocab.NilLangRef: vocab.Content(Anonymous)},
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
	if vocab.NilType.Match(p.Type) {
		p.Type = vocab.PersonType
	}

	if a.HasMetadata() {
		if p.Summary.Count() == 0 && len(a.Metadata.Blurb) > 0 {
			p.Summary = make(vocab.NaturalLanguageValues)
			_ = p.Summary.Set(vocab.NilLangRef, vocab.Content(a.Metadata.Blurb))
		}
		if p.Icon == nil && len(a.Metadata.Icon.URI) > 0 {
			avatar := vocab.ObjectNew(vocab.ImageType)
			avatar.MediaType = vocab.MimeType(a.Metadata.Icon.MimeType)
			avatar.URL = vocab.IRI(a.Metadata.Icon.URI)
			p.Icon = avatar
		}
	}

	if p.PreferredUsername.Count() == 0 {
		p.PreferredUsername = make(vocab.NaturalLanguageValues)
		_ = p.PreferredUsername.Set(vocab.NilLangRef, vocab.Content(a.Handle))
	}

	if a.Hash.IsValid() {
		if p.ID == "" {
			p.ID = vocab.ID(a.Metadata.ID)
		}
		if p.Name.Count() == 0 && a.Metadata.Name != "" {
			p.Name = make(vocab.NaturalLanguageValues)
			_ = p.Name.Set(vocab.DefaultLang, vocab.Content(a.Metadata.Name))
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

func (r *repository) loadAccountsFollowers(ctx context.Context, acc *Account) error {
	if !acc.HasMetadata() || len(acc.Metadata.FollowersIRI) == 0 || acc.AP() == nil {
		return nil
	}

	ac := acc.AP()
	result, err := r.b.SearchInCollection(followers(ac.GetLink()))
	if err != nil {
		return err
	}
	for _, res := range result {
		fol, ok := res.(vocab.Item)
		if !ok {
			continue
		}
		if !vocab.ActorTypes.Match(fol.GetType()) {
			continue
		}
		p := Account{}
		if err := p.FromActivityPub(fol); err == nil && p.IsValid() {
			acc.Followers = append(acc.Followers, p)
		}
	}
	return nil
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
	res, err := r.b.SearchInCollection(vocab.Followers.Of(ac).GetLink())
	if err != nil {
		return err
	}

	for _, li := range res {
		fol, ok := li.(vocab.Item)
		if !ok {
			continue
		}
		if !vocab.ActorTypes.Match(fol.GetType()) {
			continue
		}
		p := Account{}
		if err := p.FromActivityPub(fol); err == nil && p.IsValid() {
			acc.Following = append(acc.Following, p)
		}
	}
	return nil
}

func getItemUpdatedTime(it vocab.Item) time.Time {
	var updated time.Time
	_ = vocab.OnObject(it, func(ob *vocab.Object) error {
		updated = ob.Updated
		return nil
	})
	return updated
}

func (r *repository) loadAccountsOutbox(ctx context.Context, acc *Account) error {
	if !acc.HasMetadata() || len(acc.Metadata.OutboxIRI) == 0 || acc.AP() == nil {
		return nil
	}

	now := time.Now().UTC()
	lastUpdated := acc.Metadata.OutboxUpdated
	if now.Sub(lastUpdated)-5*time.Minute < 0 {
		return nil
	}

	ac := acc.AP()
	validTypes := append(vocab.ActivityVocabularyTypes{vocab.LikeType}, vocab.CreateType, vocab.DeleteType)
	check := []filters.Check{
		filters.HasType(validTypes...),
		filters.SameAttributedTo(ac.GetLink()),
		filters.WithMaxCount(200),
	}

	result, err := r.b.Search(check...)
	if err != nil {
		return err
	}
	for _, res := range result {
		it, ok := res.(vocab.Item)
		if !ok {
			continue
		}
		acc.Metadata.Outbox = append(acc.Metadata.Outbox, it)
		//_ = vocab.OnActivity(it, func(a *vocab.Activity) error {
		//	typ := it.GetType()
		//	if typ == vocab.CreateType {
		//		ob := a.Object
		//		if ob == nil {
		//			return nil
		//		}
		//		if ob.IsObject() {
		//			if ValidActorTypes.Contains(ob.GetType()) {
		//				act := Account{}
		//				_ = act.FromActivityPub(a)
		//				acc.children = append(act.children, &act)
		//			}
		//		}
		//	}
		//	if ValidModerationActivityTypes.Contains(typ) {
		//		m := ModerationOp{}
		//		_ = m.FromActivityPub(a)
		//		if m.Object != nil {
		//			if m.Object.Type() != ActorType {
		//				return nil
		//			}
		//			dude, dok := m.Object.(*Account)
		//			if !dok {
		//				return nil
		//			}
		//			if typ == vocab.BlockType {
		//				acc.Blocked = append(acc.Blocked, *dude)
		//			}
		//			if typ == vocab.IgnoreType {
		//				acc.Ignored = append(acc.Ignored, *dude)
		//			}
		//		}
		//	}
		//	return nil
		//})
	}
	acc.Metadata.OutboxUpdated = now
	return nil
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
	inReplyTo := make([]filters.Check, 0, len(repliesTo))
	for _, rr := range repliesTo {
		inReplyTo = append(inReplyTo, filters.SameInReplyTo(rr.GetLink()))
	}

	allReplies := make(ItemCollection, 0)
	checks := filters.All(
		filters.HasType(ValidContentTypes...),
		filters.Any(inReplyTo...),
	)

	repl, err := r.b.Search(checks)
	if err != nil {
		r.errFn()(err.Error())
	}
	for _, rr := range repl {
		if it, ok := rr.(vocab.Item); ok {
			ob := Item{}
			if err := ob.FromActivityPub(it); err == nil {
				allReplies = append(allReplies, ob)
			}
		}
	}
	// TODO(marius): probably we can thread the replies right here
	return allReplies, nil
}

func likesFilter(iris vocab.IRIs, types ...vocab.ActivityVocabularyType) filters.Check {
	byIRIChecks := make(filters.Checks, 0, len(iris))

	for _, iri := range iris {
		byIRIChecks = append(byIRIChecks, filters.SameIRI(iri))
	}

	return filters.All(
		filters.HasType(types...),
		filters.Object(filters.Any(byIRIChecks...)),
	)
}

func irisFromItems(items ...Item) vocab.IRIs {
	iris := make(vocab.IRIs, 0)
	for _, it := range items {
		if it.Deleted() {
			continue
		}
		_ = iris.Append(it.AP())
	}
	return iris
}

func (r *repository) loadItemsVotes(ctx context.Context, items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}

	activeAppreciationTypes := vocab.ActivityVocabularyTypes{vocab.LikeType}
	if Instance.Conf.DownvotingEnabled {
		activeAppreciationTypes = ValidAppreciationTypes
	}

	searches := likesFilter(irisFromItems(items...), activeAppreciationTypes...)
	votes := make(VoteCollection, 0)
	results, err := r.b.Search(searches)
	for _, res := range results {
		it, ok := res.(vocab.Item)
		if !ok {
			continue
		}
		vv := Vote{}
		if err := vv.FromActivityPub(it); err != nil {
			continue
		}
		votes = append(votes, vv)
	}

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

func AccountIRIChecks(accounts ...Account) filters.Check {
	iris := make(vocab.IRIs, 0, len(accounts))
	for _, ac := range accounts {
		if it := ac.AP(); !vocab.IsNil(it) {
			_ = iris.Append(it.GetLink())
		}
	}
	filter := make(filters.Checks, 0, len(accounts))
	for _, i := range iris {
		filter = append(filter, filters.SameIRI(i))
	}
	return filters.Any(filter...)
}

func (r *repository) loadAccountsAuthors(ctx context.Context, accounts ...Account) (AccountCollection, error) {
	if len(accounts) == 0 {
		return accounts, nil
	}

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

	authors, err := r.accounts(ctx, AccountIRIChecks(creators...))
	if err != nil {
		return accounts, errors.Annotatef(err, "unable to load accounts authors")
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

	authors, _ := r.accounts(ctx, AccountIRIChecks(submitters...))
	for _, remoteAcc := range remoteSubmitters {
		it, err := r.fedbox.Actor(ctx, remoteAcc.AP().GetLink())
		if err != nil {
			continue
		}
		_ = remoteAcc.FromActivityPub(it)
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

func (r *repository) loadModerationFollowups(ctx context.Context, items RenderableList) (ModerationRequests, error) {
	inReplyTos := make(vocab.IRIs, 0, len(items))
	for _, it := range items {
		if iri := it.AP().GetLink(); !inReplyTos.Contains(iri) {
			inReplyTos = append(inReplyTos, iri)
		}
	}

	checks := make(filters.Checks, 0, len(inReplyTos))
	for _, iri := range inReplyTos {
		checks = append(checks, filters.SameInReplyTo(iri))
	}
	followups, err := r.b.Search(filters.HasType(vocab.DeleteType, vocab.UpdateType), filters.Any(checks...))
	if err != nil {
		return nil, err
	}

	modFollowups := make(ModerationRequests, 0, len(followups))
	for _, li := range followups {
		ob, oka := li.(vocab.Item)
		if !oka {
			continue
		}
		m := new(ModerationOp)
		if err := m.FromActivityPub(ob); err != nil {
			continue
		}
		if !modFollowups.Contains(*m) {
			modFollowups = append(modFollowups, *m)
		}
	}

	return modFollowups, nil
}

func (r *repository) loadModerationDetails(ctx context.Context, items ...ModerationOp) ([]ModerationOp, error) {
	if len(items) == 0 {
		return items, nil
	}

	iris := make(vocab.IRIs, 0, len(items)*2)
	for _, it := range items {
		if it.Object != nil && it.Object.AP() != nil {
			if iri := it.Object.AP().GetLink(); !iris.Contains(iri) {
				iris = append(iris, iri)
			}
		}
		if it.SubmittedBy != nil && it.SubmittedBy.AP() != nil {
			if iri := it.SubmittedBy.AP().GetLink(); !iris.Contains(iri) {
				iris = append(iris, iri)
			}
		}
	}

	checks := make(filters.Checks, 0, len(iris))
	for _, iri := range iris {
		checks = append(checks, filters.SameIRI(iri))
	}

	result, err := r.b.Search(filters.Any(checks...))
	if err != nil {
		return items, err
	}

	for _, li := range result {
		ob, oka := li.(vocab.Item)
		if !oka {
			continue
		}
		for i, mod := range items {
			if ValidContentTypes.Match(ob.GetType()) {
				it := Item{}
				if err := it.FromActivityPub(ob); err == nil {
					if renderablesEqual(mod.Object, &it) {
						mod.Object = &it
					}
				}
			}
			if ValidActorTypes.Match(ob.GetType()) {
				auth := Account{}
				if err := auth.FromActivityPub(ob); err == nil {
					if renderablesEqual(mod.Object, &auth) {
						mod.Object = &auth
					}
					if renderablesEqual(mod.SubmittedBy, &auth) {
						mod.SubmittedBy = &auth
					}
				}
			}
			items[i] = mod
		}
	}

	return items, nil
}

func renderablesEqual(r1, r2 Renderable) bool {
	if r1 == nil || r2 == nil {
		return false
	}
	it1 := r1.AP()
	it2 := r2.AP()
	if vocab.IsNil(it1) || vocab.IsNil(it2) {
		return r1.ID() == r2.ID()
	}
	return it1.GetLink().Equals(it2.GetLink(), true)
}

func itemsEqual(i1, i2 Item) bool {
	it1 := i1.AP()
	it2 := i2.AP()
	if vocab.IsNil(it1) || vocab.IsNil(it2) {
		return i1.Hash == i2.Hash
	}
	return it1.GetLink().Equals(it2.GetLink(), true)
}

func accountsEqual(a1, a2 Account) bool {
	it1 := a1.AP()
	it2 := a2.AP()
	if vocab.IsNil(it1) || vocab.IsNil(it2) {
		return a1.Hash == a2.Hash
	}
	return it1.GetLink().Equals(it2.GetLink(), true)
}

func baseIRI(iri vocab.IRI) vocab.IRI {
	u, _ := iri.URL()
	u.Path = ""
	return vocab.IRI(u.String())
}

func getAuthors(items ItemCollection) vocab.IRIs {
	accounts := make(vocab.IRIs, 0, len(items))
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

	return accounts
}

func (r *repository) loadItemsAuthors(ctx context.Context, items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}

	accounts := getAuthors(items)
	if len(accounts) == 0 {
		return items, nil
	}

	checks := make(filters.Checks, 0, len(accounts))
	for _, auth := range accounts {
		checks = append(checks, filters.SameIRI(auth.GetLink()))
	}

	found, err := r.b.Search(filters.Any(checks...))
	if err != nil {
		r.errFn()(err.Error())
	}

	authors := make(AccountCollection, 0)
	for _, auth := range found {
		it, ok := auth.(vocab.Item)
		if !ok {
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

func getCollectionPrevNext(col vocab.ItemCollection) (prev, next vocab.IRI) {
	if len(col) > 0 {
		prev = col.First().GetLink()
	}
	if len(col) > 1 {
		next = col[len(col)-1].GetLink()
	}
	return prev, next
}

func (r *repository) account(ctx context.Context, ff ...filters.Check) (*Account, error) {
	accounts, err := r.accounts(ctx, ff...)
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

func accumulateAccountsFromCollection(col vocab.CollectionInterface) (AccountCollection, vocab.IRIs, error) {
	accounts := make(AccountCollection, 0)
	deferredTagLoads := make(vocab.IRIs, 0)
	for _, it := range col.Collection() {
		if !it.IsObject() || !ValidActorTypes.Match(it.GetType()) {
			continue
		}
		a := Account{}
		if err := a.FromActivityPub(it); err == nil && a.IsValid() {
			if len(a.Metadata.Tags) > 0 && deferredTagLoads != nil {
				for _, t := range a.Metadata.Tags {
					if t.Name == "" && t.Metadata.ID != "" {
						deferredTagLoads = append(deferredTagLoads, vocab.IRI(t.Metadata.ID))
					}
				}
			}
			accounts = append(accounts, a)
		}
	}
	return accounts, deferredTagLoads, nil
}

func assignTagsToAccounts(accounts AccountCollection, col vocab.ItemCollection) error {
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

func (r *repository) accountsFromRemote(ctx context.Context, ff ...filters.Check) (AccountCollection, error) {
	accounts := make(AccountCollection, 0)
	res, err := r.b.Search(ff...)
	if err != nil {
		return accounts, err
	}

	deferredTagLoads := make(vocab.IRIs, 0)
	col := make(vocab.ItemCollection, 0, len(res))
	for _, li := range res {
		if it, ok := li.(vocab.Item); ok {
			col = append(col, it)
		}
	}

	acc, tags, err := accumulateAccountsFromCollection(&col)
	for _, a := range acc {
		if !accounts.Contains(a) {
			accounts = append(accounts, a)
		}
	}
	deferredTagLoads = append(deferredTagLoads, tags...)

	tagSearches := make(filters.Checks, 0, len(deferredTagLoads))
	for _, tag := range deferredTagLoads {
		tagSearches = append(tagSearches, filters.SameIRI(tag))
	}

	rest, err := r.b.Search(tagSearches...)
	if err != nil {
		return accounts, err
	}

	ttt := make(vocab.ItemCollection, 0, len(rest))
	for _, tt := range rest {
		if t, ok := tt.(vocab.Item); ok {
			ttt = append(ttt, t)
		}
	}
	return accounts, assignTagsToAccounts(accounts, ttt)
}

func (r *repository) accounts(ctx context.Context, ff ...filters.Check) (AccountCollection, error) {
	return r.accountsFromRemote(ctx, ff...)
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
			_ = vocab.OnItemCollection(prop, func(col *vocab.ItemCollection) error {
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

	_ = vocab.OnObject(it, func(o *vocab.Object) error {
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

// LoadSearches loads all elements from checks
// Iterating over the activities in the resulting collections, we gather the objects and accounts
func (r *repository) LoadSearches(ctx context.Context, deps deps, checks ...filters.Check) (Cursor, error) {
	items := make(ItemCollection, 0)
	follows := make(FollowRequests, 0)
	accounts := make(AccountCollection, 0)
	moderations := make(ModerationRequests, 0)
	appreciations := make(VoteCollection, 0)
	relations := sync.Map{}

	deferredRemote := make(vocab.IRIs, 0)

	result := make(RenderableList, 0)
	resM := new(sync.RWMutex)

	results, err := r.b.Search(checks...)
	if err != nil {
		return emptyCursor, err
	}
	for _, res := range results {
		it, ok := res.(vocab.Item)
		if !ok {
			continue
		}
		typ := it.GetType()
		switch {
		case ValidContentTypes.Match(typ):
			err = vocab.OnObject(it, func(o *vocab.Object) error {
				i := Item{}
				if err := i.FromActivityPub(o); err != nil {
					return err
				}
				items = append(items, i)
				relations.Store(o.GetLink(), o.GetLink())
				return nil
			})
		case ValidActorTypes.Match(typ):
			err = vocab.OnActor(it, func(a *vocab.Actor) error {
				act := Account{}
				if err := act.FromActivityPub(a); err != nil {
					return err
				}
				accounts = append(accounts, act)
				relations.Store(a.GetLink(), a.GetLink())
				return nil
			})
		case vocab.IntransitiveActivityTypes.Match(typ), vocab.ActivityTypes.Match(typ):
			err = vocab.OnActivity(it, func(a *vocab.Activity) error {
				if typ == vocab.CreateType {
					ob := a.Object
					if ob == nil {
						return errors.Newf("nil activity object")
					}
					if vocab.IsObject(ob) {
						if ValidContentTypes.Match(ob.GetType()) {
							i := Item{}
							if err := i.FromActivityPub(a); err != nil {
								return err
							}
							items = append(items, i)
						}
						if ValidActorTypes.Match(ob.GetType()) {
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
				if ValidModerationActivityTypes.Match(typ) {
					m := ModerationOp{}
					if err := m.FromActivityPub(a); err != nil {
						return err
					}
					moderations = append(moderations, m)
					relations.Store(a.GetLink(), a.GetLink())
				}
				if ValidAppreciationTypes.Match(typ) {
					v := Vote{}
					if err := v.FromActivityPub(a); err != nil {
						return err
					}
					appreciations = append(appreciations, v)
					relations.Store(a.GetLink(), a.GetLink())
				}
				return nil
			})
		}
		if err == nil {
			for _, rem := range accumulateItemIRIs(it, deps) {
				if !deferredRemote.Contains(rem) {
					deferredRemote = append(deferredRemote, rem)
				}
			}
		}
	}

	if err != nil {
		return emptyCursor, err
	}

	next, prev := getCollectionPrevNext(results)
	if len(deferredRemote) > 0 {
		searchIRIs := make(filters.Checks, 0, len(deferredRemote))
		for _, iri := range deferredRemote {
			searchIRIs = append(searchIRIs, filters.SameIRI(iri))
		}

		res, _ := r.b.Search(filters.Any(searchIRIs...))
		for _, li := range res {
			ob, ok := li.(vocab.Item)
			if !ok || vocab.IsNil(ob) {
				continue
			}
			typ := ob.GetType()
			if vocab.ActorTypes.Match(typ) {
				ac := Account{}
				if err := ac.FromActivityPub(ob); err == nil {
					accounts = append(accounts, ac)
				}
			}
			if vocab.ObjectTypes.Match(typ) {
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
		after:  next,
		before: prev,
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
	_ = r.loadAPItem(o, *v.Item)
	act := &vocab.Activity{
		Type:  vocab.UndoType,
		Actor: author.GetLink(),
	}
	act.To, act.Bto, act.CC, act.BCC = r.defaultRecipientsList(v.SubmittedBy.Pub, Instance.Conf.PublicVotingEnabled)
	if Instance.Conf.PublicVotingEnabled {
		// NOTE(marius): if public voting is enabled we can append the recipients of the voted on object
		_ = vocab.OnObject(act, func(ob *vocab.Object) error {
			return vocab.OnObject(v.Item.Pub, func(p *vocab.Object) error {
				_ = appendRecipients(&ob.To, p.To)
				_ = appendRecipients(&ob.Bto, p.Bto)
				_ = appendRecipients(&ob.CC, p.CC)
				_ = appendRecipients(&ob.BCC, p.BCC)
				return nil
			})
		})
	}

	if exists.HasMetadata() {
		act.Object = vocab.IRI(exists.Metadata.IRI)
		i, it, err := r.ToOutbox(ctx, v.SubmittedBy.Credentials(), act)
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
	iri, it, err = r.ToOutbox(ctx, v.SubmittedBy.Credentials(), act)
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
	return errors.WrapWithStatus(err.Code, nil, "%s", err.Message)
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

	remoteWebFinger := make(map[string][]string, 0)
	checks := make(filters.Checks, 0)
	checks = append(checks, filters.HasType(vocab.PersonType))
	for _, m := range incoming {
		// TODO(marius): we need to make a distinction between FedBOX remote servers and Webfinger remote servers
		u, err := url.ParseRequestURI(m.URL)
		if err != nil {
			continue
		}
		h := fmt.Sprintf("%s://%s", u.Scheme, u.Hostname())
		if strings.Contains(m.URL, "@"+m.Name) {
			// use WebFinger
			remoteWebFinger[h] = append(remoteWebFinger[h], m.Name+"@"+u.Hostname())
			continue
		}
		if strings.Contains(r.SelfURL, h) {
			h = r.fedbox.conf.BaseURL.String()
		}

		checks = append(checks, filters.IRILike(h), filters.NameIs(m.Name))
	}

	col, err := r.b.Search(checks...)
	if err != nil {
		return nil
	}
	for _, li := range col {
		it, ok := li.(vocab.Item)
		if !ok {
			continue
		}
		for i, t := range incoming {
			_ = vocab.OnActor(it, func(act *vocab.Actor) error {
				if strings.ToLower(t.Name) == strings.ToLower(act.Name.First().String()) ||
					strings.ToLower(t.Name) == strings.ToLower(act.PreferredUsername.First().String()) {
					u := act.ID.String()
					if act.URL != nil {
						u = act.URL.GetLink().String()
					}
					incoming[i].Metadata = &ItemMetadata{ID: act.ID.String(), URL: u}
					incoming[i].URL = u
				}
				return nil
			})
		}
	}

	for h, accts := range remoteWebFinger {
		for _, acct := range accts {
			act, err := r.loadWebfingerActorFromIRI(context.TODO(), h, acct)
			if err != nil {
				r.errFn(log.Ctx{"err": err, "host": h, "account": acct})("unable to load account")
				continue
			}

			for i, t := range incoming {
				if strings.ToLower(t.Name) == strings.ToLower(act.Name.First().String()) ||
					strings.ToLower(t.Name) == strings.ToLower(act.PreferredUsername.First().String()) {
					u := act.ID.String()
					if act.URL != nil {
						u = act.URL.GetLink().String()
					}
					incoming[i].Metadata = &ItemMetadata{ID: act.ID.String(), URL: u}
					incoming[i].URL = u
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

	tagNames := make(filters.Checks, 0, len(incoming))
	for _, t := range incoming {
		check := filters.Tag(
			filters.Any(
				filters.NameIs(t.Name),
				filters.NameIs("#"+t.Name),
			),
		)
		tagNames = append(tagNames, check)
	}

	tags, _, err := r.LoadTags(ctx, tagNames...)
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
			_ = art.CC.Append(parAuth.Pub.GetLink())
		}
		par = p.Parent
	}

	*art = *r.loadAPPerson(*it)
	if it.Parent != nil {
		_ = vocab.OnObject(it.Parent.AP(), func(p *vocab.Object) error {
			_ = appendRecipients(&art.To, p.To)
			_ = appendRecipients(&art.Bto, p.Bto)
			_ = appendRecipients(&art.CC, p.CC)
			_ = appendRecipients(&art.BCC, p.BCC)
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

	_ = r.loadAPItem(art, *it)
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
	if mod.Object.ID() != AnonymousHash {
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
	_ = vocab.OnObject(toDelete, func(ob *vocab.Object) error {
		act.To = ob.To
		act.Bto = ob.Bto
		act.CC = ob.CC
		act.BCC = ob.BCC
		return nil
	})

	i, tombstone, err := r.ToOutbox(ctx, author.Credentials(), act)
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
		_ = to.Append(vocab.PublicNS)
		if act != nil {
			_ = cc.Append(vocab.Followers.IRI(act))
		}
		_ = bcc.Append(r.fedbox.Service().ID)
	}
	// NOTE(marius): we publish the activity just to the instance and its followers
	_ = cc.Append(r.app.Pub.GetLink(), vocab.Followers.IRI(r.app.Pub))

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
	_ = r.loadAPItem(art, it)
	if it.Parent != nil {
		_ = loadFromParent(art, it.Parent.AP())
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
			r.errFn(log.Ctx{"item": it.Hash, "err": err})("unable to delete item with no ID")
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
	i, ob, err = r.ToOutbox(ctx, it.SubmittedBy.Credentials(), act)
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

func (r *repository) LoadTags(ctx context.Context, ff ...filters.Check) (TagCollection, uint, error) {
	tags := make(TagCollection, 0)
	var count uint = 0

	res, err := r.b.Search(ff...)
	if err != nil {
		return nil, 0, err
	}

	for _, li := range res {
		it, ok := li.(vocab.Item)
		if !ok {
			continue
		}
		tag := Tag{}
		if err = tag.FromActivityPub(it); err != nil {
			r.errFn(log.Ctx{"type": fmt.Sprintf("%T", it)})(err.Error())
			continue
		}
		count++
		tags = append(tags, tag)
	}

	return tags, count, nil
}

type CollectionFilterFn func(context.Context, ...client.FilterFn) (vocab.CollectionInterface, error)

func (r *repository) ValidateRemoteAccount(ctx context.Context, acc *Account) error {
	now := time.Now().UTC()
	lastUpdated := acc.Metadata.OutboxUpdated
	if now.Sub(lastUpdated)-5*time.Minute < 0 {
		return nil
	}

	ltx := log.Ctx{"handle": acc.Handle, "hash": acc.Hash}
	r.infoFn(ltx)("loading account details")

	it, err := r.fedbox.Actor(ctx, acc.AP().GetLink())
	if err != nil {
		return err
	}

	a := Account{}
	if err = a.FromActivityPub(it); err != nil {
		return err
	}
	if !a.IsValid() || !accountsEqual(a, *acc) {
		return errors.Errorf("invalid actor")
	}
	return nil
}

func (r *repository) LoadAccountDetails(ctx context.Context, acc *Account) error {
	var err error
	ltx := log.Ctx{"handle": acc.Handle, "hash": acc.Hash}
	r.infoFn(ltx)("loading account details")

	if err = r.loadAccountsOutbox(ctx, acc); err != nil {
		r.infoFn(ltx, log.Ctx{"err": err.Error()})("unable to load outbox")
	}
	//if len(acc.Followers) == 0 {
	//	// TODO(marius): this needs to be moved to where we're handling all Inbox activities, not on page load
	//	if err = r.loadAccountsFollowers(ctx, acc); err != nil {
	//		r.infoFn(ltx, log.Ctx{"err": err.Error()})("unable to load followers")
	//	}
	//}
	//if len(acc.Following) == 0 {
	//	if err = r.loadAccountsFollowing(ctx, acc); err != nil {
	//		r.infoFn(ltx, log.Ctx{"err": err.Error()})("unable to load following")
	//	}
	//}
	return err
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
	err = r.ValidateRemoteAccount(ctx, acc)
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
	_ = appendRecipients(&response.To, vocab.IRI(er.Metadata.ID))
	response.Type = vocab.RejectType
	if accept {
		response.Type = vocab.AcceptType
	}
	if reason != nil {
		_ = r.loadAPItem(response, *reason)
	}
	response.Object = vocab.IRI(f.Metadata.ID)
	response.Actor = vocab.IRI(ed.Metadata.ID)

	i, it, err := r.ToOutbox(ctx, f.SubmittedBy.Credentials(), response)
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
	if err := r.FollowActor(ctx, er, ed, reason); err != nil {
		r.errFn(log.Ctx{
			"err":      err.Error(),
			"follower": er.Handle,
			"followed": ed.Handle,
		})("Unable to follow")
		return err
	}
	return nil
}

func (r *repository) FollowActor(ctx context.Context, er, ed Account, reason *Item) error {
	follower := r.loadAPPerson(er)
	followed := r.loadAPPerson(ed)

	follow := new(vocab.Follow)
	follow.To, _, follow.CC, follow.BCC = r.defaultRecipientsList(follower, true)
	_ = appendRecipients(&follow.To, followed.GetLink())
	if reason != nil {
		_ = r.loadAPItem(follow, *reason)
	}
	follow.Type = vocab.FollowType
	follow.Object = followed.GetLink()
	follow.Actor = follower.GetLink()
	_, _, err := r.ToOutbox(ctx, er.Credentials(), follow)
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
	i, ap, err := r.ToOutbox(ctx, a.Credentials(), act)
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

func (r *repository) moderationActivity(ctx context.Context, er *vocab.Actor, ed vocab.Item, reason *Item) (*vocab.Activity, error) {
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

func (r *repository) moderationActivityOnItem(ctx context.Context, er Account, ed Item, reason *Item) (*vocab.Activity, error) {
	reporter := r.loadAPPerson(er)
	reported := new(vocab.Object)
	r.loadAPItem(reported, ed)
	if !accountValidForC2S(&er) {
		return nil, errors.Unauthorizedf("invalid account %s", er.Handle)
	}
	return r.moderationActivity(ctx, reporter, reported, reason)
}

func (r *repository) moderationActivityOnAccount(ctx context.Context, er, ed Account, reason *Item) (*vocab.Activity, error) {
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
	i, ob, err := r.ToOutbox(ctx, er.Credentials(), block)

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
	i, ob, err := r.ToOutbox(ctx, er.Credentials(), block)
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
	i, ob, err := r.ToOutbox(ctx, er.Credentials(), flag)
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
	i, ob, err := r.ToOutbox(ctx, er.Credentials(), flag)
	if err != nil {
		r.errFn()(err.Error())
		return err
	}
	r.cache.removeRelated(i, ob, flag)
	return nil
}

func (r *repository) loadItemFromCacheOrIRI(ctx context.Context, iri vocab.IRI) (vocab.Item, error) {
	if it := r.cache.get(iri); !vocab.IsNil(it) {
		if getItemUpdatedTime(it).Sub(time.Now()) < 10*time.Minute {
			return it, nil
		}
	}
	return r.fedbox.Client(nil).CtxLoadIRI(ctx, iri)
}

func (r *repository) loadCollectionFromCacheOrIRI(ctx context.Context, iri vocab.IRI) (vocab.CollectionInterface, bool, error) {
	if it := r.cache.get(cacheKey(iri, ContextAccount(ctx))); !vocab.IsNil(it) {
		if c, okCol := it.(vocab.CollectionInterface); okCol && getItemUpdatedTime(it).Sub(time.Now()) < 10*time.Minute && c.Count() > 0 {
			return c, true, nil
		}
	}
	col, err := r.fedbox.collection(ctx, iri)
	return col, false, err
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
