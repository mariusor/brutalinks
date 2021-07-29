package app

import (
	"context"
	"crypto"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/errors"
	"github.com/go-ap/handlers"
	j "github.com/go-ap/jsonld"
	"github.com/mariusor/go-littr/internal/log"
	"github.com/mariusor/qstring"
	"github.com/spacemonkeygo/httpsig"
	"golang.org/x/sync/errgroup"
)

var nilIRI = EqualsString("-")
var nilIRIs = CompStrs{nilIRI}

var notNilIRI = DifferentThanString("-")
var notNilIRIs = CompStrs{notNilIRI}

type repository struct {
	SelfURL string
	app     *Account
	fedbox  *fedbox
	infoFn  CtxLogFn
	errFn   CtxLogFn
}

func (r repository) BaseURL() pub.IRI {
	return r.fedbox.baseURL
}

// Repository middleware
func (h handler) Repository(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), RepositoryCtxtKey, h.storage)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func ActivityPubService(c appConfig) (*repository, error) {
	pub.ItemTyperFunc = pub.GetItemByType

	infoFn := func(ctx ...log.Ctx) LogFn {
		return c.Logger.WithContext(append(ctx, log.Ctx{"client": "api"})...).Debugf
	}
	errFn := func(ctx ...log.Ctx) LogFn {
		return c.Logger.WithContext(append(ctx, log.Ctx{"client": "api"})...).Warnf
	}
	ua := fmt.Sprintf("%s-%s", c.HostName, Instance.Version)

	repo := &repository{
		SelfURL: c.BaseURL,
		infoFn:  infoFn,
		errFn:   errFn,
	}
	var err error
	repo.fedbox, err = NewClient(
		SetURL(c.APIURL),
		SetInfoLogger(infoFn),
		SetErrorLogger(errFn),
		SetUA(ua),
		SkipTLSCheck(!c.Env.IsProd()),
	)
	if err != nil {
		return repo, err
	}
	return repo, nil
}

func accountURL(acc Account) pub.IRI {
	return pub.IRI(fmt.Sprintf("%s%s", Instance.BaseURL, AccountLocalLink(&acc)))
}

func BuildIDFromItem(i Item) (pub.ID, bool) {
	if !i.IsValid() {
		return "", false
	}
	if i.HasMetadata() && len(i.Metadata.ID) > 0 {
		return pub.ID(i.Metadata.ID), true
	}
	return "", false
}

func BuildActorID(a Account) pub.ID {
	if !a.IsValid() {
		return pub.PublicNS
	}
	return pub.ID(a.Metadata.ID)
}

func loadAPItem(it pub.Item, item Item) error {
	return pub.OnObject(it, func(o *pub.Object) error {
		if id, ok := BuildIDFromItem(item); ok {
			o.ID = id
		}
		if item.MimeType == MimeTypeURL {
			o.Type = pub.PageType
			if item.Hash.IsValid() {
				o.URL = pub.ItemCollection{
					pub.IRI(item.Data),
					pub.IRI(ItemLocalLink(&item)),
				}
			} else {
				o.URL = pub.IRI(item.Data)
			}
		} else {
			wordCount := strings.Count(item.Data, " ") +
				strings.Count(item.Data, "\t") +
				strings.Count(item.Data, "\n") +
				strings.Count(item.Data, "\r\n")
			if wordCount > 300 {
				o.Type = pub.ArticleType
			} else {
				o.Type = pub.NoteType
			}

			if item.Hash.IsValid() {
				o.URL = pub.IRI(ItemLocalLink(&item))
			}
			o.Name = make(pub.NaturalLanguageValues, 0)
			switch item.MimeType {
			case MimeTypeMarkdown:
				o.Source.MediaType = pub.MimeType(item.MimeType)
				o.MediaType = MimeTypeHTML
				if item.Data != "" {
					o.Source.Content.Set("en", pub.Content(item.Data))
					o.Content.Set("en", pub.Content(Markdown(item.Data)))
				}
			case MimeTypeText:
				fallthrough
			case MimeTypeHTML:
				o.MediaType = pub.MimeType(item.MimeType)
				o.Content.Set("en", pub.Content(item.Data))
			}
		}

		o.Published = item.SubmittedAt
		o.Updated = item.UpdatedAt

		if item.Deleted() {
			del := pub.Tombstone{
				ID:         o.ID,
				Type:       pub.TombstoneType,
				FormerType: o.Type,
				Deleted:    o.Updated,
			}
			repl := make(pub.ItemCollection, 0)
			if item.Parent != nil {
				if par, ok := BuildIDFromItem(*item.Parent); ok {
					repl = append(repl, par)
				}
				if item.OP == nil {
					item.OP = item.Parent
				}
			}
			if item.OP != nil {
				if op, ok := BuildIDFromItem(*item.OP); ok {
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
			o.Name.Set("en", pub.Content(item.Title))
		}
		if item.SubmittedBy != nil {
			o.AttributedTo = BuildActorID(*item.SubmittedBy)
		}

		to := make(pub.ItemCollection, 0)
		bcc := make(pub.ItemCollection, 0)
		cc := make(pub.ItemCollection, 0)
		repl := make(pub.ItemCollection, 0)

		if item.Parent != nil {
			p := item.Parent
			first := true
			for {
				if par, ok := BuildIDFromItem(*p); ok {
					repl = append(repl, par)
				}
				if p.SubmittedBy.IsValid() {
					if pAuth := BuildActorID(*p.SubmittedBy); !pub.PublicNS.Equals(pAuth, true) {
						if first {
							if !to.Contains(pAuth) {
								to = append(to, pAuth)
							}
							first = false
						} else if !cc.Contains(pAuth) {
							cc = append(cc, pAuth)
						}
					}
				}
				if p.Parent == nil {
					break
				}
				p = p.Parent
			}
		}
		if item.OP != nil {
			if op, ok := BuildIDFromItem(*item.OP); ok {
				o.Context = op
			}
		}
		if len(repl) > 0 {
			o.InReplyTo = repl
		}

		// TODO(marius): add proper dynamic recipients to this based on some selector in the frontend
		if !item.Private() {
			to = append(to, pub.PublicNS)
		}
		if item.Metadata != nil {
			m := item.Metadata
			for _, rec := range m.To {
				mto := pub.IRI(rec.Metadata.ID)
				if !to.Contains(mto) {
					to = append(to, mto)
				}
			}
			for _, rec := range m.CC {
				mcc := pub.IRI(rec.Metadata.ID)
				if !cc.Contains(mcc) {
					cc = append(cc, mcc)
				}
			}
			if m.Mentions != nil || m.Tags != nil {
				o.Tag = make(pub.ItemCollection, 0)
				for _, men := range m.Mentions {
					// todo(marius): retrieve object ids of each mention and add it to the CC of the object
					t := pub.Mention{
						Type: pub.MentionType,
						Name: pub.NaturalLanguageValues{{Ref: pub.NilLangRef, Value: pub.Content(men.Name)}},
						Href: pub.IRI(men.URL),
					}
					if men.Metadata != nil && len(men.Metadata.ID) > 0 {
						t.ID = pub.IRI(men.Metadata.ID)
					}
					o.Tag.Append(t)
				}
				for _, tag := range m.Tags {
					t := pub.Object{
						URL:  pub.ID(tag.URL),
						To:   pub.ItemCollection{pub.PublicNS},
						Name: pub.NaturalLanguageValues{{Ref: pub.NilLangRef, Value: pub.Content("#" + tag.Name)}},
					}
					if tag.Metadata != nil && len(tag.Metadata.ID) > 0 {
						t.ID = pub.IRI(tag.Metadata.ID)
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

var anonymousActor = &pub.Actor{
	ID:                pub.PublicNS,
	Name:              pub.NaturalLanguageValues{{pub.NilLangRef, pub.Content(Anonymous)}},
	Type:              pub.PersonType,
	PreferredUsername: pub.NaturalLanguageValues{{pub.NilLangRef, pub.Content(Anonymous)}},
}

func anonymousPerson(url pub.IRI) *pub.Actor {
	p := anonymousActor
	p.Inbox = handlers.Inbox.IRI(url)
	return p
}

func (r *repository) loadAPPerson(a Account) *pub.Actor {
	var p *pub.Actor
	if act, ok := a.pub.(*pub.Actor); ok {
		p = act
	} else {
		p = new(pub.Actor)
	}
	if p.Type == "" {
		p.Type = pub.PersonType
	}

	if a.HasMetadata() {
		if p.Summary.Count() == 0 && a.Metadata.Blurb != nil && len(a.Metadata.Blurb) > 0 {
			p.Summary = pub.NaturalLanguageValuesNew()
			p.Summary.Set(pub.NilLangRef, a.Metadata.Blurb)
		}
		if p.Icon == nil && len(a.Metadata.Icon.URI) > 0 {
			avatar := pub.ObjectNew(pub.ImageType)
			avatar.MediaType = pub.MimeType(a.Metadata.Icon.MimeType)
			avatar.URL = pub.IRI(a.Metadata.Icon.URI)
			p.Icon = avatar
		}
	}

	if p.PreferredUsername.Count() == 0 {
		p.PreferredUsername = pub.NaturalLanguageValuesNew()
		p.PreferredUsername.Set(pub.NilLangRef, pub.Content(a.Handle))
	}

	if a.Hash.IsValid() {
		if p.ID == "" {
			p.ID = pub.ID(a.Metadata.ID)
		}
		if p.Name.Count() == 0 && a.Metadata.Name != "" {
			p.Name = pub.NaturalLanguageValuesNew()
			p.Name.Set("en", pub.Content(a.Metadata.Name))
		}
		if p.Inbox == nil && len(a.Metadata.InboxIRI) > 0 {
			p.Inbox = pub.IRI(a.Metadata.InboxIRI)
		}
		if p.Outbox == nil && len(a.Metadata.OutboxIRI) > 0 {
			p.Outbox = pub.IRI(a.Metadata.OutboxIRI)
		}
		if p.Liked == nil && len(a.Metadata.LikedIRI) > 0 {
			p.Liked = pub.IRI(a.Metadata.LikedIRI)
		}
		if p.Followers == nil && len(a.Metadata.FollowersIRI) > 0 {
			p.Followers = pub.IRI(a.Metadata.FollowersIRI)
		}
		if p.Following == nil && len(a.Metadata.FollowingIRI) > 0 {
			p.Following = pub.IRI(a.Metadata.FollowingIRI)
		}
		if p.URL == nil && len(a.Metadata.URL) > 0 {
			p.URL = pub.IRI(a.Metadata.URL)
		}
		if p.Endpoints == nil && r.fedbox.Service().Endpoints != nil {
			p.Endpoints = &pub.Endpoints{
				SharedInbox:                r.fedbox.Service().Inbox,
				OauthAuthorizationEndpoint: r.fedbox.Service().Endpoints.OauthAuthorizationEndpoint,
				OauthTokenEndpoint:         r.fedbox.Service().Endpoints.OauthTokenEndpoint,
			}
		}
	}

	if p.PublicKey.ID == "" && a.IsValid() && a.HasMetadata() && a.Metadata.Key != nil && a.Metadata.Key.Public != nil {
		p.PublicKey = pub.PublicKey{
			ID:           pub.ID(fmt.Sprintf("%s#main-key", p.ID)),
			Owner:        p.ID,
			PublicKeyPem: fmt.Sprintf("-----BEGIN PUBLIC KEY-----\n%s\n-----END PUBLIC KEY-----", base64.StdEncoding.EncodeToString(a.Metadata.Key.Public)),
		}
	}
	return p
}

func getSigner(pubKeyID string, key crypto.PrivateKey) *httpsig.Signer {
	hdrs := []string{"(request-target)", "host", "date"}
	return httpsig.NewSigner(pubKeyID, key, httpsig.RSASHA256, hdrs)
}

// @todo(marius): the decision which sign function to use (the one for S2S or the one for C2S)
//   should be made in fedbox, because that's the place where we know if the request we're signing
//   is addressed to an IRI belonging to that specific fedbox instance or to another ActivityPub server
func (r *repository) WithAccount(a *Account) *repository {
	r.fedbox.SignBy(a)
	return r
}

func (r *repository) LoadItem(ctx context.Context, iri pub.IRI) (Item, error) {
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

func (r *repository) loadAccountsVotes(ctx context.Context, accounts ...Account) (AccountCollection, error) {
	if len(accounts) == 0 {
		return accounts, nil
	}
	for _, account := range accounts {
		err := r.loadAccountVotes(ctx, &account, nil)
		if err != nil {
			r.errFn()(err.Error())
		}
	}
	return accounts, nil
}

func accountInCollection(ac Account, col AccountCollection) bool {
	for _, fol := range col {
		if fol.Hash == ac.Hash {
			return true
		}
	}
	return false
}

func (r *repository) loadAccountsFollowers(ctx context.Context, acc *Account) error {
	if !acc.HasMetadata() || len(acc.Metadata.FollowersIRI) == 0 {
		return nil
	}
	collFn := func(ctx context.Context, f *Filters) (pub.CollectionInterface, error) {
		return r.fedbox.Collection(ctx, pub.IRI(acc.Metadata.FollowersIRI), Values(f))
	}
	return LoadFromCollection(ctx, collFn, &colCursor{filters: &Filters{}}, func(o pub.CollectionInterface) error {
		for _, fol := range o.Collection() {
			if !pub.ActorTypes.Contains(fol.GetType()) {
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

func (r *repository) loadAccountsFollowing(ctx context.Context, acc *Account) error {
	if !acc.HasMetadata() || len(acc.Metadata.FollowersIRI) == 0 {
		return nil
	}
	collFn := func(ctx context.Context, f *Filters) (pub.CollectionInterface, error) {
		return r.fedbox.Collection(ctx, pub.IRI(acc.Metadata.FollowingIRI), Values(f))
	}
	return LoadFromCollection(ctx, collFn, &colCursor{filters: &Filters{}}, func(o pub.CollectionInterface) error {
		for _, fol := range o.Collection() {
			if !pub.ActorTypes.Contains(fol.GetType()) {
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

var (
	ocTypes = pub.ActivityVocabularyTypes{pub.OrderedCollectionType, pub.OrderedCollectionPageType}
	cTypes  = pub.ActivityVocabularyTypes{pub.CollectionType, pub.CollectionPageType}
)

func (r *repository) loadAccountsOutbox(ctx context.Context, acc *Account) error {
	if !acc.HasMetadata() || len(acc.Metadata.OutboxIRI) == 0 {
		return nil
	}
	collFn := func(ctx context.Context, f *Filters) (pub.CollectionInterface, error) {
		return r.fedbox.Collection(ctx, pub.IRI(acc.Metadata.OutboxIRI), Values(f))
	}
	latest := time.Now().Add(-6 * 30 * 24 * time.Hour).UTC()
	max := MaxContentItems * 25 // NOTE(marius): this affects how big the session stored value for an account can get
	return LoadFromCollection(ctx, collFn, &colCursor{filters: &Filters{MaxItems: max}}, func(o pub.CollectionInterface) error {
		if ocTypes.Contains(o.GetType()) {
			pub.OnOrderedCollection(o, func(oc *pub.OrderedCollection) error {
				acc.Metadata.OutboxUpdated = oc.Updated
				return nil
			})
		}
		if cTypes.Contains(o.GetType()) {
			pub.OnCollection(o, func(c *pub.Collection) error {
				acc.Metadata.OutboxUpdated = c.Updated
				return nil
			})
		}
		for _, it := range o.Collection() {
			skipOutbox := false
			typ := it.GetType()
			if ValidAppreciationTypes.Contains(typ) {
				v := new(Vote)
				if err := v.FromActivityPub(it); err == nil && !acc.Votes.Contains(*v) {
					acc.Votes = append(acc.Votes, *v)
					skipOutbox = true
				}
			}
			if ValidModerationActivityTypes.Contains(typ) {
				p := new(Account)
				if err := p.FromActivityPub(it); err != nil && !p.IsValid() {
					continue
				}
				if typ == pub.BlockType {
					acc.Blocked = append(acc.Blocked, *p)
				}
				if typ == pub.IgnoreType {
					acc.Ignored = append(acc.Ignored, *p)
				}
				skipOutbox = true
			}
			pub.OnActivity(it, func(a *pub.Activity) error {
				skipOutbox = skipOutbox || a.Updated.Sub(latest) > 0
				return nil
			})
			if len(acc.Metadata.Outbox) < max && !skipOutbox {
				acc.Metadata.Outbox = append(acc.Metadata.Outbox, pub.FlattenProperties(it))
			}
		}
		return nil
	})
}

func getRepliesOf(items ...Item) pub.IRIs {
	repliesTo := make(pub.IRIs, 0)
	iriFn := func(it Item) pub.IRI {
		if it.pub != nil {
			return it.pub.GetLink()
		}
		if id, ok := BuildIDFromItem(it); ok {
			return id
		}
		return ""
	}
	for _, it := range items {
		if it.OP.IsValid() {
			it = *it.OP
		}
		iri := iriFn(it)
		if len(iri) > 0 && !repliesTo.Contains(iri) {
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
	f := &Filters{}
	for _, top := range repliesTo {
		collFn := func(ctx context.Context, f *Filters) (pub.CollectionInterface, error) {
			return r.fedbox.Replies(ctx, top.GetLink(), Values(f))
		}
		err := LoadFromCollection(ctx, collFn, &colCursor{filters: f}, func(c pub.CollectionInterface) error {
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
	}
	// TODO(marius): probably we can thread the replies right here
	return allReplies, nil
}

func (r *repository) loadAccountVotes(ctx context.Context, acc *Account, items ItemCollection) error {
	if acc == nil || acc.pub == nil {
		return nil
	}
	f := &Filters{
		Object: &Filters{
			IRI: ItemHashFilter(items...),
		},
		Type: AppreciationActivitiesFilter,
	}
	collFn := func(ctx context.Context, f *Filters) (pub.CollectionInterface, error) {
		return r.fedbox.Outbox(ctx, acc.pub, Values(f))
	}
	return LoadFromCollection(ctx, collFn, &colCursor{filters: f}, func(col pub.CollectionInterface) error {
		for _, it := range col.Collection() {
			if !it.IsObject() || !ValidAppreciationTypes.Contains(it.GetType()) {
				continue
			}
			v := new(Vote)
			if err := v.FromActivityPub(it); err == nil && !acc.Votes.Contains(*v) {
				acc.Votes = append(acc.Votes, *v)
			}
		}
		return nil
	})
}

func (r *repository) loadItemsVotes(ctx context.Context, items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}
	voteActivities := pub.ActivityVocabularyTypes{pub.LikeType, pub.DislikeType, pub.UndoType}
	f := &Filters{
		Object: &Filters{},
		Type:   ActivityTypesFilter(voteActivities...),
	}
	for _, it := range items {
		f.Object.IRI = append(f.Object.IRI, LikeString(it.Hash.String()))
	}
	collFn := func(ctx context.Context, f *Filters) (pub.CollectionInterface, error) {
		return r.fedbox.Inbox(ctx, r.fedbox.Service(), Values(f))
	}
	err := LoadFromCollection(ctx, collFn, &colCursor{filters: f}, func(c pub.CollectionInterface) error {
		for _, vAct := range c.Collection() {
			if !vAct.IsObject() || !voteActivities.Contains(vAct.GetType()) {
				continue
			}
			v := new(Vote)
			if err := v.FromActivityPub(vAct); err == nil {
				for k, ob := range items {
					if itemsEqual(*v.Item, ob) {
						items[k].Score += v.Weight
					}
				}
			}
		}
		return nil
	})
	return items, err
}

func EqualsString(s string) CompStr {
	return CompStr{Operator: "=", Str: s}
}

func ItemHashFilter(items ...Item) CompStrs {
	filter := make(CompStrs, 0)
	for _, it := range items {
		if !it.Hash.IsValid() {
			continue
		}
		hash := LikeString(it.Hash.String())
		if len(hash.Str) == 0 || filter.Contains(hash) {
			continue
		}
		filter = append(filter, hash)
	}
	return filter
}

func AccountHashFilter(accounts ...Account) CompStrs {
	filter := make(CompStrs, 0)
	for _, ac := range accounts {
		var f CompStr
		if ac.pub == nil || ac.pub.GetLink().Contains(pub.IRI(Instance.Conf.APIURL), false) {
			if !ac.Hash.IsValid() {
				continue
			}
			f = LikeString(ac.Hash.String())
		} else {
			f = EqualsString(ac.pub.GetLink().String())
		}
		if len(f.Str) == 0 || filter.Contains(f) {
			continue
		}
		filter = append(filter, f)
	}
	return filter
}

func ActivityTypesFilter(t ...pub.ActivityVocabularyType) CompStrs {
	r := make(CompStrs, len(t))
	for i, typ := range t {
		r[i] = EqualsString(string(typ))
	}
	return r
}

func (r *repository) loadAccountsAuthors(ctx context.Context, accounts ...Account) ([]Account, error) {
	if len(accounts) == 0 {
		return accounts, nil
	}
	fActors := Filters{
		Type: ActivityTypesFilter(ValidActorTypes...),
	}
	creators := make([]Account, 0)
	for _, ac := range accounts {
		creators = append(creators, *ac.CreatedBy)
	}
	fActors.IRI = AccountHashFilter(creators...)
	if len(fActors.IRI) == 0 {
		return accounts, errors.Errorf("unable to load accounts authors")
	}
	authors, err := r.accounts(ctx, &fActors)
	if err != nil {
		return accounts, errors.Annotatef(err, "unable to load accounts authors")
	}
	for k, ac := range accounts {
		found := false
		for i, auth := range authors {
			if !auth.IsValid() {
				continue
			}
			if accountsEqual(*ac.CreatedBy, auth) {
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
	for _, it := range items {
		if sub := *it.SubmittedBy; sub.IsValid() && !submitters.Contains(sub) {
			submitters = append(submitters, sub)
		}
	}
	fActors := Filters{
		Type: ActivityTypesFilter(ValidActorTypes...),
		IRI:  AccountHashFilter(submitters...),
	}

	if len(fActors.IRI) == 0 {
		return items, errors.Errorf("unable to load items authors")
	}
	authors, err := r.accounts(ctx, &fActors)
	if err != nil {
		return items, errors.Annotatef(err, "unable to load items authors")
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
	inReplyTo := make(pub.IRIs, 0)
	for _, it := range items {
		iri := it.AP().GetLink()
		if !inReplyTo.Contains(iri) {
			inReplyTo = append(inReplyTo, iri)
		}
	}

	modActions := new(Filters)
	modActions.Type = ActivityTypesFilter(pub.DeleteType, pub.UpdateType)
	modActions.InReplTo = IRIsFilter(inReplyTo...)
	modActions.Actor = &Filters{
		IRI: notNilIRIs,
	}
	act, err := r.fedbox.Outbox(ctx, r.fedbox.Service(), Values(modActions))
	if err != nil {
		return nil, err
	}
	modFollowups := make(ModerationRequests, 0)
	err = pub.OnCollectionIntf(act, func(c pub.CollectionInterface) error {
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
	return AccountHashFilter(accounts...)
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
		_, hash := path.Split(it.Object.AP().GetLink().String())
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
				if it.Object != nil && it.Object.AP().GetLink().Equals(auth.pub.GetLink(), false) {
					it.Object = &authors[i]
				}
			}
			for i, obj := range objects {
				if it.Object != nil && it.Object.AP().GetLink().Equals(obj.pub.GetLink(), false) {
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

func itemsEqual(i1, i2 Item) bool {
	return i1.Hash == i2.Hash
}

func accountsEqual(a1, a2 Account) bool {
	return a1.Hash == a2.Hash || (len(a1.Handle)+len(a2.Handle) > 0 && a1.Handle == a2.Handle)
}

func (r *repository) loadItemsAuthors(ctx context.Context, items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}

	accounts := make(map[pub.IRI]AccountCollection)

	fActors := &Filters{
		Type: ActivityTypesFilter(ValidActorTypes...),
	}

	baseIRI := func (iri pub.IRI) pub.IRI {
		u, _ := iri.URL()
		u.Path = ""
		return pub.IRI(u.String())
	}
	for _, it := range items {
		if it.SubmittedBy.IsValid() {
			iri := baseIRI(it.SubmittedBy.AP().GetLink())
			// Adding an item's author to the list of accounts we want to load from the ActivityPub API
			accounts[iri] = append(accounts[iri], *it.SubmittedBy)
		}
		if it.HasMetadata() {
			// Adding an item's recipients list (To and CC) to the list of accounts we want to load from the ActivityPub API
			if len(it.Metadata.To) > 0 && it.SubmittedBy.AP() != nil {
				iri := baseIRI(it.SubmittedBy.AP().GetLink())
				accounts[iri] = append(accounts[iri], it.Metadata.To...)
			}
			if len(it.Metadata.CC) > 0 {
				for _, cc := range it.Metadata.CC {
					if cc.AP() == nil {
						continue
					}
					iri := baseIRI(cc.AP().GetLink())
					accounts[iri] = append(accounts[iri], it.Metadata.CC...)
				}
			}
		}
	}

	authors := make(AccountCollection, 0)
	if len(accounts) > 0 {
		searches := make(RemoteLoads)
		for i, acc := range accounts {
			ff := fActors
			ff.IRI = AccountHashFilter(acc...)
			f := &Filters{ Object: ff }
			f.Type = ActivityTypesFilter(pub.CreateType)
			actorsCol := func (a pub.Item, f ...client.FilterFn) pub.IRI {
				return iri(actors.IRI(a), f...)
			}
			searches[i] = []RemoteLoad{
				{
					loadFn:  actorsCol,
					filters: []*Filters{ff},
				},
				{
					loadFn: inbox,
					filters: []*Filters{f},
				},
			}

			err := LoadFromSearches(r, searches, func(col pub.CollectionInterface) error {
				for _, it := range col.Collection() {
					acc := Account{Metadata: &AccountMetadata{}}
					if err := acc.FromActivityPub(it); err != nil {
						r.errFn(log.Ctx{"type": fmt.Sprintf("%T", it)})(err.Error())
						continue
					}
					if acc.IsValid() && !authors.Contains(acc) {
						authors = append(authors, acc)
					}
				}
				return nil
			})
			if err != nil {
				return items, errors.Annotatef(err, "unable to load items authors")
			}
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
		}
		col = append(col, it)
	}
	return col, nil
}

func getCollectionPrevNext(col pub.CollectionInterface) (prev, next string) {
	qFn := func(i pub.Item) url.Values {
		if i == nil {
			return url.Values{}
		}
		if u, err := i.GetLink().URL(); err == nil {
			return u.Query()
		}
		return url.Values{}
	}
	beforeFn := func(i pub.Item) string {
		return qFn(i).Get("before")
	}
	afterFn := func(i pub.Item) string {
		return qFn(i).Get("after")
	}
	nextFromLastFn := func(i pub.Item) string {
		if u, err := i.GetLink().URL(); err == nil {
			_, next = path.Split(u.Path)
			return next
		}
		return ""
	}
	switch col.GetType() {
	case pub.OrderedCollectionPageType:
		if c, ok := col.(*pub.OrderedCollectionPage); ok {
			prev = beforeFn(c.Prev)
			next = afterFn(c.Next)
		}
	case pub.OrderedCollectionType:
		if c, ok := col.(*pub.OrderedCollection); ok {
			next = afterFn(c.First)
			if next == "" && len(c.OrderedItems) > 0 {
				next = nextFromLastFn(c.OrderedItems[len(c.OrderedItems)-1])
			}
		}
	case pub.CollectionPageType:
		if c, ok := col.(*pub.CollectionPage); ok {
			prev = beforeFn(c.Prev)
			next = afterFn(c.Next)
		}
	case pub.CollectionType:
		if c, ok := col.(*pub.Collection); ok {
			next = afterFn(c.First)
			if next == "" && len(c.Items) > 0 {
				next = nextFromLastFn(c.Items[len(c.Items)-1])
			}
		}
	}
	return prev, next
}

type res struct {
	status accumStatus
	err    error
}

type accumStatus int8

const (
	accumError    accumStatus = -1
	accumContinue accumStatus = iota
	accumSuccess
	accumEndOfCollection
)

// LoadFromCollection iterates over a collection returned by the f function, until accum is satisfied
func LoadFromCollection(ctx context.Context, loadColFn CollectionFn, cur *colCursor, accum func(pub.CollectionInterface) error) error {
	var err error
	processed := 0
	for {
		var status bool
		var col pub.CollectionInterface

		if col, err = loadColFn(ctx, cur.filters); err != nil {
			return err
		}

		var prev string
		if err = pub.OnCollectionIntf(col, accum); err != nil {
			return err
		}
		prev, cur.filters.Next = getCollectionPrevNext(col)
		if processed == 0 {
			cur.filters.Prev = prev
		}
		processed += len(col.Collection())
		st := accumContinue
		if len(cur.filters.Next) == 0 || uint(processed) == col.Count() {
			st = accumEndOfCollection
		}
		if status && processed >= cur.filters.MaxItems {
			st = accumSuccess
		}
		if st != accumContinue {
			break
		}
	}

	return err
}

func (r *repository) account(ctx context.Context, ff *Filters) (Account, error) {
	accounts, err := r.accounts(ctx, ff)
	if err != nil {
		return AnonymousAccount, err
	}
	if len(accounts) == 0 {
		return AnonymousAccount, errors.NotFoundf("account not found")
	}
	if len(accounts) > 1 {
		return AnonymousAccount, errors.BadRequestf("too many accounts found")
	}
	return accounts[0], nil
}

func (r *repository) accounts(ctx context.Context, ff ...*Filters) ([]Account, error) {
	actors := func(ctx context.Context, f *Filters) (pub.CollectionInterface, error) {
		return r.fedbox.Actors(ctx, Values(f))
	}
	accounts := make([]Account, 0)
	// TODO(marius): see how we can use the context returned by errgroup.WithContext()
	g, _ := errgroup.WithContext(ctx)
	for _, f := range ff {
		g.Go(func() error {
			return LoadFromCollection(ctx, actors, &colCursor{filters: f}, func(col pub.CollectionInterface) error {
				for _, it := range col.Collection() {
					if !it.IsObject() || !ValidActorTypes.Contains(it.GetType()) {
						continue
					}
					a := Account{}
					if err := a.FromActivityPub(it); err == nil && a.IsValid() {
						accounts = append(accounts, a)
					}
				}
				// TODO(marius): this needs to be externalized also to a different function that we can pass from outer scope
				//   This function implements the logic for breaking out of the collection iteration cycle and returns a bool
				return nil
			})
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *repository) object(ctx context.Context, ff *Filters) (Item, error) {
	objects, err := r.objects(ctx, ff)
	if err != nil {
		return Item{}, err
	}
	if len(objects) == 0 {
		return Item{}, errors.NotFoundf("object not found")
	}
	if len(objects) > 1 {
		return Item{}, errors.BadRequestf("too many objects found")
	}
	return objects[0], nil

}

func (r *repository) objects(ctx context.Context, ff ...*Filters) (ItemCollection, error) {
	objects := func(ctx context.Context, f *Filters) (pub.CollectionInterface, error) {
		return r.fedbox.Objects(ctx, Values(f))
	}
	items := make(ItemCollection, 0)
	// TODO(marius): see how we can use the context returned by errgroup.WithContext()
	g, _ := errgroup.WithContext(ctx)
	for _, f := range ff {
		g.Go(func() error {
			return LoadFromCollection(ctx, objects, &colCursor{filters: f}, func(c pub.CollectionInterface) error {
				for _, it := range c.Collection() {
					i := new(Item)
					if err := i.FromActivityPub(it); err == nil && i.IsValid() {
						items = append(items, *i)
					}
				}
				return nil
			})
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var err error
	if items, err = r.loadItemsAuthors(ctx, items...); err != nil {
		return nil, err
	}
	if items, err = r.loadItemsVotes(ctx, items...); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *repository) Objects(ctx context.Context, ff ...*Filters) (Cursor, error) {
	items, err := r.objects(ctx, ff...)
	if err != nil {
		return emptyCursor, err
	}
	result := make(RenderableList, 0)
	for _, it := range items {
		if it.Hash.IsValid() {
			result.Append(&it)
		}
	}
	var next, prev Hash
	for _, f := range ff {
		next = HashFromString(f.Next)
		prev = HashFromString(f.Prev)
	}
	return Cursor{
		after:  next,
		before: prev,
		items:  result,
		total:  uint(len(result)),
	}, nil
}

func validFederated(i Item, f *Filters) bool {
	ob, err := pub.ToObject(i.pub)
	if err != nil {
		return false
	}
	if len(f.Generator) > 0 {
		for _, g := range f.Generator {
			if i.pub == nil || ob.Generator == nil {
				continue
			}
			if g == nilIRI {
				if ob.Generator.GetLink().Equals(pub.IRI(Instance.BaseURL), false) {
					return false
				}
				return true
			}
			if ob.Generator.GetLink().Equals(pub.IRI(g.Str), false) {
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
			if pub.IRI(r.Str).Equals(pub.PublicNS, false) && i.Private() {
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

func IRIsFilter(iris ...pub.IRI) CompStrs {
	r := make(CompStrs, len(iris))
	for i, iri := range iris {
		r[i] = EqualsString(iri.String())
	}
	return r
}

// ActorCollection loads the service's collection returned by fn.
// First step is to load the Create activities from the inbox
// Iterating over the activities in the resulting collection, we gather the objects and accounts
//  With the resulting Object IRIs we load from the objects collection with our matching filters
//  With the resulting Actor IRIs we load from the accounts collection with matching filters
func (r *repository) ActorCollection(ctx context.Context, fn CollectionFn, ff ...*Filters) (Cursor, error) {
	items := make(ItemCollection, 0)
	follows := make(FollowRequests, 0)
	accounts := make(AccountCollection, 0)
	moderations := make(ModerationRequests, 0)
	appreciations := make(VoteCollection, 0)
	relations := make(map[pub.IRI]pub.IRI)
	relM := new(sync.RWMutex)

	deferredItems := make(CompStrs, 0)
	deferredActors := make(CompStrs, 0)
	deferredActivities := make(CompStrs, 0)

	appendToDeferred := func(ob pub.Item, filterFn func(string) CompStr) {
		if ob.IsObject() {
			return
		}
		iri := filterFn(ob.GetLink().String())
		if strings.Contains(iri.String(), string(actors)) && !deferredActors.Contains(iri) {
			deferredActors = append(deferredActors, iri)
		}
		if strings.Contains(iri.String(), string(objects)) && !deferredItems.Contains(iri) {
			deferredItems = append(deferredItems, iri)
		}
		if strings.Contains(iri.String(), string(activities)) && !deferredActivities.Contains(iri) {
			deferredActivities = append(deferredActivities, iri)
		}
	}
	result := make(RenderableList, 0)
	resM := new(sync.RWMutex)
	// TODO(marius): see how we can use the context returned by errgroup.WithContext()
	g, _ := errgroup.WithContext(ctx)
	for j := range ff {
		f := ff[j]
		g.Go(func() error {
			err := LoadFromCollection(ctx, fn, &colCursor{filters: f}, func(col pub.CollectionInterface) error {
				for _, it := range col.Collection() {
					pub.OnActivity(it, func(a *pub.Activity) error {
						relM.Lock()
						defer relM.Unlock()

						typ := it.GetType()
						if typ == pub.CreateType {
							ob := a.Object
							if ob == nil {
								return nil
							}
							if ob.IsObject() {
								if ValidContentTypes.Contains(ob.GetType()) {
									i := Item{}
									i.FromActivityPub(ob)
									if validItem(i, f) {
										items = append(items, i)
									}
								}
								if ValidActorTypes.Contains(ob.GetType()) {
									a := Account{}
									a.FromActivityPub(ob)
									accounts = append(accounts, a)
								}
							} else {
								i := Item{}
								i.FromActivityPub(a)
								appendToDeferred(ob, EqualsString)
							}
							relations[a.GetLink()] = ob.GetLink()
						}
						if it.GetType() == pub.FollowType {
							f := FollowRequest{}
							f.FromActivityPub(a)
							follows = append(follows, f)
							relations[a.GetLink()] = a.GetLink()
							appendToDeferred(a.Object, EqualsString)
						}
						if ValidModerationActivityTypes.Contains(typ) {
							m := ModerationOp{}
							m.FromActivityPub(a)
							moderations = append(moderations, m)
							relations[a.GetLink()] = a.GetLink()
							appendToDeferred(a.Object, EqualsString)
						}
						if ValidAppreciationTypes.Contains(typ) {
							v := Vote{}
							v.FromActivityPub(a)
							appreciations = append(appreciations, v)
							relations[a.GetLink()] = a.GetLink()
						}
						return nil
					})
				}
				// TODO(marius): this needs to be externalized also to a different function that we can pass from outer scope
				//   This function implements the logic for breaking out of the collection iteration cycle and returns a bool
				return nil
			})
			if err != nil {
				return err
			}
			if len(deferredItems) > 0 {
				if f.Object == nil {
					f.Object = new(Filters)
				}
				ff := f.Object
				ff.IRI = deferredItems
				objects, _ := r.objects(ctx, ff)
				for _, d := range objects {
					if !d.IsValid() {
						continue
					}
					if !items.Contains(d) {
						items = append(items, d)
					}
				}
			}
			if len(deferredActors) > 0 {
				if f.Actor == nil {
					f.Actor = new(Filters)
				}
				ff := f.Actor
				ff.IRI = deferredActors
				acc, _ := r.accounts(ctx, ff)
				for _, d := range acc {
					if !d.IsValid() {
						continue
					}
					if !accounts.Contains(d) {
						accounts = append(accounts, d)
					}
				}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return emptyCursor, err
	}
	var err error
	items, err = r.loadItemsAuthors(ctx, items...)
	if err != nil {
		return emptyCursor, err
	}
	items, err = r.loadItemsVotes(ctx, items...)
	if err != nil {
		return emptyCursor, err
	}
	_, err = r.loadItemsReplies(ctx, items...)
	if err != nil {
		return emptyCursor, err
	}
	follows, err = r.loadFollowsAuthors(ctx, follows...)
	if err != nil {
		return emptyCursor, err
	}
	accounts, err = r.loadAccountsAuthors(ctx, accounts...)
	if err != nil {
		return emptyCursor, err
	}
	moderations, err = r.loadModerationDetails(ctx, moderations...)
	if err != nil {
		return emptyCursor, err
	}

	relM.RLock()
	defer relM.RUnlock()
	resM.Lock()
	defer resM.Unlock()
	for _, rel := range relations {
		for i := range items {
			it := items[i]
			if it.IsValid() && it.pub.GetLink() == rel {
				result.Append(&it)
			}
		}
		for i := range follows {
			f := follows[i]
			if f.pub != nil && f.pub.GetLink() == rel {
				result.Append(&f)
			}
		}
		for i := range accounts {
			a := accounts[i]
			if a.pub != nil && a.pub.GetLink() == rel {
				result.Append(&a)
			}
		}
		for i := range moderations {
			a := moderations[i]
			if rel.Equals(a.AP().GetLink(), false) {
				result.Append(&a)
			}
		}
		for i := range appreciations {
			a := appreciations[i]
			if a.pub != nil && a.pub.GetLink() == rel {
				result.Append(&a)
			}
		}
	}

	var next, prev Hash
	for _, f := range ff {
		if len(f.Next) > 0 {
			next = HashFromString(f.Next)
		}
		if len(f.Prev) > 0 {
			prev = HashFromString(f.Prev)
		}
	}

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

	url := fmt.Sprintf("%s/%s", v.Item.Metadata.ID, "likes")
	itemVotes, err := r.loadVotesCollection(ctx, pub.IRI(url), pub.IRI(v.SubmittedBy.Metadata.ID))
	// first step is to verify if vote already exists:
	if err != nil {
		r.errFn(log.Ctx{
			"url": url,
			"err": err,
		})(err.Error())
	}
	var exists Vote
	for _, vot := range itemVotes {
		if !vot.SubmittedBy.IsValid() || !v.SubmittedBy.IsValid() {
			continue
		}
		if vot.SubmittedBy.Hash == v.SubmittedBy.Hash {
			exists = vot
			break
		}
	}

	o := new(pub.Object)
	loadAPItem(o, *v.Item)
	act := &pub.Activity{
		Type:  pub.UndoType,
		To:    pub.ItemCollection{pub.PublicNS},
		BCC:   pub.ItemCollection{r.fedbox.Service().ID},
		Actor: author.GetLink(),
	}

	if exists.HasMetadata() {
		act.Object = pub.IRI(exists.Metadata.IRI)
		if _, _, err := r.fedbox.ToOutbox(ctx, act); err != nil {
			r.errFn()(err.Error())
		}
	}

	if v.Weight > 0 && exists.Weight <= 0 {
		act.Type = pub.LikeType
		act.Object = o.GetLink()
	}
	if v.Weight < 0 && exists.Weight >= 0 {
		act.Type = pub.DislikeType
		act.Object = o.GetLink()
	}
	if v.Item.SubmittedBy != nil {
		auth := v.Item.SubmittedBy.AP()
		if !auth.GetLink().Contains(r.BaseURL(), false) {
			// NOTE(marius): this assumes that the instance the user is from has a shared inbox at {instance_hostname}/inbox
			u, _ := auth.GetLink().URL()
			u.Path = ""
			act.BCC = append(act.BCC, pub.IRI(u.String()))
		}
		act.To = append(act.To, auth.GetLink())
	}

	var (
		iri pub.IRI
		it pub.Item
	)
	iri, it, err = r.fedbox.ToOutbox(ctx, act)
	if err != nil {
		r.errFn()(err.Error())
		return v, err
	}
	r.infoFn(log.Ctx{"act": iri, "obj": it.GetLink(), "type": it.GetType()})("saved activity")
	err = v.FromActivityPub(act)
	return v, err
}

func (r *repository) loadVotesCollection(ctx context.Context, iri pub.IRI, actors ...pub.IRI) ([]Vote, error) {
	cntActors := len(actors)
	f := &Filters{}
	if cntActors > 0 {
		f.AttrTo = make(CompStrs, cntActors)
		for i, a := range actors {
			f.AttrTo[i] = LikeString(a.String())
		}
	}
	likes, err := r.fedbox.Collection(ctx, iri, Values(f))
	// first step is to verify if vote already exists:
	if err != nil {
		return nil, err
	}
	votes := make([]Vote, 0)
	err = pub.OnOrderedCollection(likes, func(col *pub.OrderedCollection) error {
		for _, like := range col.OrderedItems {
			vote := Vote{}
			vote.FromActivityPub(like)
			votes = append(votes, vote)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return votes, nil
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
	ap, err := pub.UnmarshalJSON(body)
	if err != nil {
		r.errFn()(err.Error())
		return it, err
	}
	err = it.FromActivityPub(ap)
	if err != nil {
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

func loadCCsFromMentions(incoming []Tag) pub.ItemCollection {
	if len(incoming) == 0 {
		return nil
	}

	iris := make(pub.ItemCollection, 0)
	for _, inc := range incoming {
		if inc.Metadata != nil {
			iris = append(iris, pub.IRI(inc.Metadata.ID))
		}
	}

	return iris
}

func loadMentionsIfExisting (r *repository, ctx context.Context, incoming TagCollection) TagCollection {
	if len(incoming) == 0 {
		return incoming
	}

	remoteFilters := make([]*Filters, 0)
	for _, m := range incoming {
		// TODO(marius): we need to make a distinction between FedBOX remote servers and Webfinger remote servers
		u, err := url.ParseRequestURI(m.URL)
		if err != nil {
			continue
		}
		host := fmt.Sprintf("%s://%s", u.Scheme, u.Hostname())
		if strings.Contains(r.SelfURL, host) {
			host = r.fedbox.baseURL.String()
		}
		urlFilter := EqualsString(host)
		var (
			filter *Filters
			add bool
		)
		for i, fil := range remoteFilters {
			if fil.URL.Contains(urlFilter) {
				filter = remoteFilters[i]
			}
		}
		if filter == nil {
			filter = &Filters{Type: ActivityTypesFilter(pub.PersonType)}
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
		actorsIRI := actors.IRI(pub.IRI(filter.IRI[0].Str))
		filter.IRI = nil
		col, err := r.fedbox.client.Collection(ctx, actorsIRI, Values(filter))
		if err != nil {
			r.errFn(log.Ctx{"err": err})("unable to load accounts from mentions")
			continue
		}
		pub.OnCollectionIntf(col, func(col pub.CollectionInterface) error {
			for _, it := range col.Collection() {
				for i, t := range incoming {
					pub.OnActor(it, func(act *pub.Actor) error {
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

	return incoming
}

func loadTagsIfExisting (r *repository, ctx context.Context, incoming TagCollection) TagCollection {
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
				strings.ToLower("#" + t.Name) == strings.ToLower(tag.Name) {
				incoming[i] = tag
			}
		}
	}
	return incoming
}

func (r *repository) SaveItem(ctx context.Context, it Item) (Item, error) {
	if it.SubmittedBy == nil || !it.SubmittedBy.HasMetadata() {
		return Item{}, errors.Newf("invalid account")
	}
	var author *pub.Actor
	if it.SubmittedBy.IsLogged() {
		author = r.loadAPPerson(*it.SubmittedBy)
	} else {
		author = anonymousPerson(r.BaseURL())
	}
	if !accountValidForC2S(it.SubmittedBy) {
		return it, errors.Unauthorizedf("invalid account %s", it.SubmittedBy.Handle)
	}

	to := make(pub.ItemCollection, 0)
	cc := make(pub.ItemCollection, 0)
	bcc := make(pub.ItemCollection, 0)

	var err error

	if it.HasMetadata() {
		m := it.Metadata
		if len(m.To) > 0 {
			for _, rec := range m.To {
				to = append(to, pub.IRI(rec.Metadata.ID))
			}
		}
		if len(m.CC) > 0 {
			for _, rec := range m.CC {
				cc = append(cc, pub.IRI(rec.Metadata.ID))
			}
		}
		m.Tags = loadTagsIfExisting(r, ctx, m.Tags)
		m.Mentions = loadMentionsIfExisting(r, ctx, m.Mentions)
		cc = append(cc, loadCCsFromMentions(m.Mentions)...)
		it.Metadata = m
	}

	if !it.Private() {
		to = append(to, pub.PublicNS)
		if it.Parent == nil && it.SubmittedBy.HasMetadata() && len(it.SubmittedBy.Metadata.FollowersIRI) > 0 {
			cc = append(cc, pub.IRI(it.SubmittedBy.Metadata.FollowersIRI))
		}
		bcc = append(bcc, r.fedbox.Service().ID)
	}

	art := new(pub.Object)
	loadAPItem(art, it)
	id := art.GetLink()

	act := &pub.Activity{
		To:     to,
		CC:     cc,
		BCC:    bcc,
		Actor:  author.GetLink(),
		Object: art,
	}
	loadAuthors := true
	if it.Deleted() {
		if len(id) == 0 {
			r.errFn(log.Ctx{
				"item": it.Hash,
			})(err.Error())
			return it, errors.NotFoundf("item hash is empty, can not delete")
		}
		act.Object = id
		act.Type = pub.DeleteType
		loadAuthors = false
	} else {
		if len(id) == 0 {
			act.Type = pub.CreateType
		} else {
			act.Type = pub.UpdateType
		}
	}
	var (
		i pub.IRI
		ob pub.Item
	)
	i, ob, err = r.fedbox.ToOutbox(ctx, act)
	if err != nil {
		r.errFn()(err.Error())
		return it, err
	}
	r.infoFn(log.Ctx{"act": i, "obj": ob.GetLink(), "type": ob.GetType()})("saved activity")
	err = it.FromActivityPub(ob)
	if err != nil {
		r.errFn()(err.Error())
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
	// TODO(marius): see how we can use the context returned by errgroup.WithContext()
	g, _ := errgroup.WithContext(ctx)
	for _, f := range ff {
		g.Go(func() error {
			it, err := r.fedbox.Objects(ctx, Values(f))
			if err != nil {
				r.errFn()(err.Error())
				return err
			}
			return pub.OnOrderedCollection(it, func(col *pub.OrderedCollection) error {
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
	if err := g.Wait(); err != nil {
		return tags, count, err
	}
	return tags, count, nil
}

type CollectionFilterFn func(context.Context, ...client.FilterFn) (pub.CollectionInterface, error)

func (r *repository) LoadAccounts(ctx context.Context, colFn CollectionFilterFn, ff ...*Filters) (AccountCollection, uint, error) {
	accounts := make(AccountCollection, 0)
	var count uint = 0

	if colFn == nil {
		colFn = r.fedbox.Actors
	}

	// TODO(marius): see how we can use the context returned by errgroup.WithContext()
	g, _ := errgroup.WithContext(ctx)
	for _, f := range ff {
		g.Go(func() error {
			it, err := colFn(ctx, Values(f))
			if err != nil {
				r.errFn()(err.Error())
				return err
			}
			return pub.OnOrderedCollection(it, func(col *pub.OrderedCollection) error {
				count = col.TotalItems
				for _, it := range col.OrderedItems {
					acc := Account{Metadata: &AccountMetadata{}}
					if err := acc.FromActivityPub(it); err != nil {
						r.errFn(log.Ctx{"type": fmt.Sprintf("%T", it)})(err.Error())
						continue
					}
					accounts = append(accounts, acc)
				}
				accounts, err = r.loadAccountsVotes(ctx, accounts...)
				return err
			})
		})
	}
	if err := g.Wait(); err != nil {
		return accounts, count, err
	}
	return accounts, count, nil
}

func (r *repository) LoadAccountDetails(ctx context.Context, acc *Account) error {
	r.WithAccount(acc)
	ltx := log.Ctx{
		"handle": acc.Handle,
		"hash":   acc.Hash,
	}
	var err error
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
	if err = r.loadAccountsOutbox(ctx, acc); err != nil {
		r.infoFn(ltx, log.Ctx{"err": err.Error()})("unable to load outbox")
	}
	return nil
}

func (r *repository) LoadAccount(ctx context.Context, iri pub.IRI) (*Account, error) {
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
		f.Type = ActivityTypesFilter(pub.FollowType)
	}
	var followReq pub.CollectionInterface
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
		return errors.Newf("invalid account to follow %s", er.Handle)
	}
	ed := f.Object
	if !accountValidForC2S(ed) {
		return errors.Unauthorizedf("invalid account for request %s", ed.Handle)
	}

	to := make(pub.ItemCollection, 0)
	bcc := make(pub.ItemCollection, 0)

	to = append(to, pub.IRI(er.Metadata.ID))
	bcc = append(bcc, r.fedbox.Service().ID)

	response := new(pub.Activity)
	if reason != nil {
		loadAPItem(response, *reason)
	}
	response.To = to
	response.Type = pub.RejectType
	response.BCC = bcc
	response.Object = pub.IRI(f.Metadata.ID)
	response.Actor = pub.IRI(ed.Metadata.ID)
	if accept {
		to = append(to, pub.PublicNS)
		response.Type = pub.AcceptType
	}

	_, _, err := r.fedbox.ToOutbox(ctx, response)
	if err != nil {
		r.errFn(log.Ctx{
			"err":      err,
			"follower": er.Handle,
			"followed": ed.Handle,
		})("unable to respond to follow")
		return err
	}
	return nil
}

func (r *repository) FollowAccount(ctx context.Context, er, ed Account, reason *Item) error {
	follower := r.loadAPPerson(er)
	followed := r.loadAPPerson(ed)
	if !accountValidForC2S(&er) {
		return errors.Unauthorizedf("invalid account %s", er.Handle)
	}

	to := make(pub.ItemCollection, 0)
	bcc := make(pub.ItemCollection, 0)

	//to = append(to, follower.GetLink())
	to = append(to, pub.PublicNS)
	bcc = append(bcc, r.fedbox.Service().ID)

	follow := new(pub.Follow)
	if reason != nil {
		loadAPItem(follow, *reason)
	}
	follow.Type = pub.FollowType
	follow.To = to
	follow.BCC = bcc
	follow.Object = followed.GetLink()
	follow.Actor = follower.GetLink()
	_, _, err := r.fedbox.ToOutbox(ctx, follow)
	if err != nil {
		r.errFn(log.Ctx{
			"err":      err,
			"follower": er.Handle,
			"followed": ed.Handle,
		})("Unable to follow")
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
	act := &pub.Activity{
		To:      pub.ItemCollection{pub.PublicNS},
		BCC:     pub.ItemCollection{fx.ID},
		Updated: now,
	}

	parent := fx
	if a.CreatedBy != nil {
		parent = r.loadAPPerson(*a.CreatedBy)
	}
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
		act.Type = pub.DeleteType
		act.Object = id
	} else {
		act.Object = p
		p.To = pub.ItemCollection{pub.PublicNS}
		p.BCC = pub.ItemCollection{fx.ID}
		if len(id) == 0 {
			act.Type = pub.CreateType
		} else {
			act.Type = pub.UpdateType
		}
	}

	var ap pub.Item
	ltx := log.Ctx{"actor": a.Handle}
	if _, ap, err = r.fedbox.ToOutbox(ctx, act); err != nil {
		ltx["parent"] = parent.GetLink()
		if ap != nil {
			ltx["activity"] = ap.GetLink()
		}
		r.errFn(ltx, log.Ctx{"err": err})("account save failed")
		return a, err
	}
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

func (r *repository) LoadAccountWithDetails(ctx context.Context, actor Account, f ...*Filters) (*Cursor, error) {
	c, err := r.LoadActorOutbox(ctx, actor.pub, f...)
	if err != nil {
		return c, err
	}
	remaining := make(RenderableList, 0)
	for _, it := range c.items {
		switch it.Type() {
		case AppreciationType:
			v, ok := it.(*Vote)
			if !ok {
				continue
			}
			if actor.Votes.Contains(*v) {
				continue
			}
			actor.Votes = append(actor.Votes, *v)
		default:
			remaining.Append(it)
		}
	}
	c.items = remaining
	c.total = uint(len(remaining))
	return c, nil
}

func (r *repository) LoadActorOutbox(ctx context.Context, actor pub.Item, f ...*Filters) (*Cursor, error) {
	if actor == nil {
		return nil, errors.Errorf("Invalid actor")
	}
	outbox := func(ctx context.Context, f *Filters) (pub.CollectionInterface, error) {
		return r.fedbox.Outbox(ctx, actor, Values(f))
	}
	cursor, err := r.ActorCollection(ctx, outbox, f...)
	if err != nil {
		return nil, err
	}
	return &cursor, nil
}

func (r *repository) LoadActivities(ctx context.Context, ff ...*Filters) (*Cursor, error) {
	collFn := func(ctx context.Context, f *Filters) (pub.CollectionInterface, error) {
		return r.fedbox.Activities(ctx, Values(f))
	}
	cursor, err := r.ActorCollection(ctx, collFn, ff...)
	if err != nil {
		return nil, err
	}
	return &cursor, nil
}

func (r *repository) LoadActorInbox(ctx context.Context, actor pub.Item, f ...*Filters) (*Cursor, error) {
	if actor == nil {
		return nil, errors.Errorf("Invalid actor")
	}
	collFn := func(ctx context.Context, f *Filters) (pub.CollectionInterface, error) {
		return r.fedbox.Inbox(ctx, actor, Values(f))
	}
	cursor, err := r.ActorCollection(ctx, collFn, f...)
	if err != nil {
		return nil, err
	}
	return &cursor, nil
}

func (r repository) moderationActivity(ctx context.Context, er *pub.Actor, ed pub.Item, reason *Item) (*pub.Activity, error) {
	bcc := make(pub.ItemCollection, 0)
	bcc = append(bcc, r.fedbox.Service().ID, r.app.pub.GetLink())

	// We need to add the ed/er accounts' creators to the CC list
	cc := make(pub.ItemCollection, 0)
	if er.AttributedTo != nil && !er.AttributedTo.GetLink().Equals(pub.PublicNS, true) {
		cc = append(cc, er.AttributedTo.GetLink())
	}
	pub.OnObject(ed, func(o *pub.Object) error {
		if o.AttributedTo != nil {
			auth, err := r.fedbox.Actor(ctx, o.AttributedTo.GetLink())
			if err == nil && auth != nil && auth.AttributedTo != nil &&
				!(auth.AttributedTo.GetLink().Equals(auth.GetLink(), false) || auth.AttributedTo.GetLink().Equals(pub.PublicNS, true)) {
				cc = append(cc, auth.AttributedTo.GetLink())
			}
		}
		return nil
	})

	act := new(pub.Activity)
	if reason != nil {
		reason.MakePrivate()
		loadAPItem(act, *reason)
	}
	act.BCC = bcc
	act.CC = cc
	act.Object = ed.GetLink()
	act.Actor = er.GetLink()
	return act, nil
}

func (r repository) moderationActivityOnItem(ctx context.Context, er Account, ed Item, reason *Item) (*pub.Activity, error) {
	reporter := r.loadAPPerson(er)
	reported := new(pub.Object)
	loadAPItem(reported, ed)
	if !accountValidForC2S(&er) {
		return nil, errors.Unauthorizedf("invalid account %s", er.Handle)
	}
	return r.moderationActivity(ctx, reporter, reported, reason)
}

func (r repository) moderationActivityOnAccount(ctx context.Context, er, ed Account, reason *Item) (*pub.Activity, error) {
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
	block.Type = pub.BlockType
	if _, _, err = r.fedbox.ToOutbox(ctx, block); err != nil {
		r.errFn()(err.Error())
		return err
	}
	return nil
}

func (r *repository) BlockItem(ctx context.Context, er Account, ed Item, reason *Item) error {
	block, err := r.moderationActivityOnItem(ctx, er, ed, reason)
	if err != nil {
		r.errFn()(err.Error())
		return err
	}
	block.Type = pub.BlockType
	if _, _, err = r.fedbox.ToOutbox(ctx, block); err != nil {
		r.errFn()(err.Error())
		return err
	}
	return nil
}

func (r *repository) ReportItem(ctx context.Context, er Account, it Item, reason *Item) error {
	flag, err := r.moderationActivityOnItem(ctx, er, it, reason)
	if err != nil {
		r.errFn()(err.Error())
		return err
	}
	flag.Type = pub.FlagType
	if _, _, err = r.fedbox.ToOutbox(ctx, flag); err != nil {
		r.errFn()(err.Error())
		return err
	}
	return nil
}

func (r *repository) ReportAccount(ctx context.Context, er, ed Account, reason *Item) error {
	flag, err := r.moderationActivityOnAccount(ctx, er, ed, reason)
	if err != nil {
		r.errFn()(err.Error())
		return err
	}
	flag.Type = pub.FlagType
	if _, _, err = r.fedbox.ToOutbox(ctx, flag); err != nil {
		r.errFn()(err.Error())
		return err
	}
	return nil
}
