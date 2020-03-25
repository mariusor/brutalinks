package app

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-ap/handlers"
	j "github.com/go-ap/jsonld"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/mariusor/qstring"
	"github.com/spacemonkeygo/httpsig"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

var nilIRI = EqualsString("-")
var nilIRIs = CompStrs{nilIRI}

type repository struct {
	BaseURL string
	SelfURL string
	app     *Account
	fedbox  *fedbox
	infoFn  LogFn
	errFn   LogFn
}

var ValidActorTypes = pub.ActivityVocabularyTypes{
	pub.PersonType,
}

var ValidItemTypes = pub.ActivityVocabularyTypes{
	pub.ArticleType,
	pub.NoteType,
	pub.LinkType,
	pub.PageType,
	pub.DocumentType,
	pub.VideoType,
	pub.AudioType,
}

var ValidActivityTypes = pub.ActivityVocabularyTypes{
	pub.CreateType,
	pub.LikeType,
	pub.FollowType,
}

// Repository middleware
func (h handler) Repository(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), RepositoryCtxtKey, h.storage)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func ActivityPubService(c appConfig) *repository {
	pub.ItemTyperFunc = pub.JSONGetItemByType

	BaseURL = c.APIURL
	ActorsURL = fmt.Sprintf("%s/%s", BaseURL, actors)
	ObjectsURL = fmt.Sprintf("%s/%s", BaseURL, objects)

	infoFn := func(s string, ctx log.Ctx) {}
	errFn := func(s string, ctx log.Ctx) {
		if ctx != nil {
			ctx = log.Ctx{"client": "api"}
		}
		c.Logger.WithContext(ctx).Error(s)
	}
	ua := fmt.Sprintf("%s-%s", Instance.HostName, Instance.Version)

	f, _ := NewClient(SetURL(BaseURL), SetInfoLogger(infoFn), SetErrorLogger(errFn), SetUA(ua))

	return &repository{
		BaseURL: c.APIURL,
		SelfURL: c.BaseURL,
		fedbox:  f,
		infoFn:  infoFn,
		errFn:   errFn,
	}
}

func getObjectType(el pub.Item) string {
	if el == nil {
		return ""
	}
	var label = ""
	switch el.(type) {
	case *pub.OrderedCollection:
		label = "collection"
	case pub.OrderedCollection:
		label = "collection"
	case pub.Person:
		if o, ok := el.(pub.Person); ok {
			label = o.Name.First().Value
		}
	case *pub.Person:
		if o, ok := el.(*pub.Person); ok {
			label = o.Name.First().Value
		}
	}
	return label
}

func BuildCollectionID(a Account, o handlers.CollectionType) pub.ID {
	if len(a.Handle) > 0 {
		return pub.ID(fmt.Sprintf("%s/%s/%s", ActorsURL, url.PathEscape(a.Hash.String()), o))
	}
	return pub.ID(fmt.Sprintf("%s/%s", BaseURL, o))
}

var BaseURL = "http://fedbox.git"
var ActorsURL = fmt.Sprintf("%s/%s", BaseURL, actors)
var ObjectsURL = fmt.Sprintf("%s/%s", BaseURL, objects)

func apAccountID(a Account) pub.ID {
	if len(a.Hash) >= 8 {
		return pub.ID(fmt.Sprintf("%s/%s", ActorsURL, a.Hash.String()))
	}
	return pub.ID(fmt.Sprintf("%s/anonymous", ActorsURL))
}

func accountURL(acc Account) pub.IRI {
	return pub.IRI(fmt.Sprintf("%s%s", Instance.BaseURL, AccountPermaLink(acc)))
}

func BuildIDFromItem(i Item) (pub.ID, bool) {
	if !i.IsValid() {
		return "", false
	}
	if i.HasMetadata() && len(i.Metadata.ID) > 0 {
		return pub.ID(i.Metadata.ID), true
	}
	return pub.ID(fmt.Sprintf("%s/%s", ObjectsURL, url.PathEscape(i.Hash.String()))), true
}

func BuildActorID(a Account) pub.ID {
	if !a.IsValid() {
		return pub.ID(pub.PublicNS)
	}
	if a.HasMetadata() && len(a.Metadata.ID) > 0 {
		return pub.ID(a.Metadata.ID)
	}
	return pub.ID(fmt.Sprintf("%s/%s", ActorsURL, url.PathEscape(a.Hash.String())))
}

func loadAPItem(item Item) pub.Item {
	o := pub.Object{}

	if id, ok := BuildIDFromItem(item); ok {
		o.ID = id
	}
	if item.MimeType == MimeTypeURL {
		o.Type = pub.PageType
		o.URL = pub.IRI(item.Data)
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

		if len(item.Hash) > 0 {
			o.URL = pub.IRI(ItemPermaLink(item))
		}
		o.Name = make(pub.NaturalLanguageValues, 0)
		switch item.MimeType {
		case MimeTypeMarkdown:
			o.Source.MediaType = pub.MimeType(item.MimeType)
			o.MediaType = pub.MimeType(MimeTypeHTML)
			if item.Data != "" {
				o.Source.Content.Set("en", item.Data)
				o.Content.Set("en", string(Markdown(item.Data)))
			}
		case MimeTypeText:
			fallthrough
		case MimeTypeHTML:
			o.MediaType = pub.MimeType(item.MimeType)
			o.Content.Set("en", item.Data)
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
				repl = append(repl, pub.IRI(par))
			}
			if item.OP == nil {
				item.OP = item.Parent
			}
		}
		if item.OP != nil {
			if op, ok := BuildIDFromItem(*item.OP); ok {
				iri := pub.IRI(op)
				del.Context = iri
				if !repl.Contains(iri) {
					repl = append(repl, iri)
				}
			}
		}
		if len(repl) > 0 {
			del.InReplyTo = repl
		}

		return del
	}

	if item.Title != "" {
		o.Name.Set("en", string(item.Title))
	}
	if item.SubmittedBy != nil {
		id := BuildActorID(*item.SubmittedBy)
		o.AttributedTo = pub.IRI(id)
	}
	repl := make(pub.ItemCollection, 0)
	if item.Parent != nil {
		p := item.Parent
		if par, ok := BuildIDFromItem(*p); ok {
			repl = append(repl, pub.IRI(par))
		}
		if p.SubmittedBy.IsValid() {
			if pAuth := BuildActorID(*p.SubmittedBy); pub.IRI(pAuth) != pub.PublicNS {
				o.To = append(o.To, pub.IRI(pAuth))
			}
		}
		if item.OP == nil {
			item.OP = p
		}
	}
	if item.OP != nil {
		if op, ok := BuildIDFromItem(*item.OP); ok {
			iri := pub.IRI(op)
			o.Context = iri
			if !repl.Contains(iri) {
				repl = append(repl, iri)
			}
		}
	}
	if len(repl) > 0 {
		o.InReplyTo = repl
	}
	to := make(pub.ItemCollection, 0)
	bcc := make(pub.ItemCollection, 0)
	cc := make(pub.ItemCollection, 0)
	// TODO(marius): add proper dynamic recipients to this based on some selector in the frontend
	if !item.Private() {
		to = append(to, pub.PublicNS)
		bcc = append(bcc, pub.IRI(BaseURL))
	}
	if item.Metadata != nil {
		m := item.Metadata
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
		if m.Mentions != nil || m.Tags != nil {
			o.Tag = make(pub.ItemCollection, 0)
			for _, men := range m.Mentions {
				// todo(marius): retrieve object ids of each mention and add it to the CC of the object
				t := pub.Object{
					ID:   pub.ID(men.URL),
					Type: pub.MentionType,
					Name: pub.NaturalLanguageValues{{Ref: pub.NilLangRef, Value: men.Name}},
				}
				o.Tag.Append(t)
			}
			for _, tag := range m.Tags {
				t := pub.Object{
					ID:   pub.ID(tag.URL),
					Type: pub.ObjectType,
					Name: pub.NaturalLanguageValues{{Ref: pub.NilLangRef, Value: tag.Name}},
				}
				o.Tag.Append(t)
			}
		}
	}
	o.To = to
	o.CC = cc
	o.BCC = bcc

	return &o
}

func anonymousActor() *pub.Actor {
	p := pub.Actor{}
	name := pub.NaturalLanguageValues{
		{pub.NilLangRef, Anonymous},
	}
	p.ID = pub.ID(pub.PublicNS)
	p.Type = pub.PersonType
	p.Name = name
	p.PreferredUsername = name
	return &p
}

func anonymousPerson(url string) *pub.Actor {
	p := anonymousActor()
	p.Inbox = pub.IRI(fmt.Sprintf("%s/inbox", url))
	return p
}

func loadAPPerson(a Account) *pub.Actor {
	p := pub.Actor{}
	p.Type = pub.PersonType
	p.Name = pub.NaturalLanguageValuesNew()
	p.PreferredUsername = pub.NaturalLanguageValuesNew()

	if a.HasMetadata() {
		if a.Metadata.Blurb != nil && len(a.Metadata.Blurb) > 0 {
			p.Summary = pub.NaturalLanguageValuesNew()
			p.Summary.Set(pub.NilLangRef, string(a.Metadata.Blurb))
		}
		if len(a.Metadata.Icon.URI) > 0 {
			avatar := pub.ObjectNew(pub.ImageType)
			avatar.MediaType = pub.MimeType(a.Metadata.Icon.MimeType)
			avatar.URL = pub.IRI(a.Metadata.Icon.URI)
			p.Icon = avatar
		}
	}

	p.PreferredUsername.Set(pub.NilLangRef, a.Handle)

	if len(a.Hash) > 0 {
		if a.IsFederated() {
			p.ID = pub.ID(a.Metadata.ID)
			p.Name.Set("en", a.Metadata.Name)
			if len(a.Metadata.InboxIRI) > 0 {
				p.Inbox = pub.IRI(a.Metadata.InboxIRI)
			}
			if len(a.Metadata.OutboxIRI) > 0 {
				p.Outbox = pub.IRI(a.Metadata.OutboxIRI)
			}
			if len(a.Metadata.LikedIRI) > 0 {
				p.Liked = pub.IRI(a.Metadata.LikedIRI)
			}
			if len(a.Metadata.FollowersIRI) > 0 {
				p.Followers = pub.IRI(a.Metadata.FollowersIRI)
			}
			if len(a.Metadata.FollowingIRI) > 0 {
				p.Following = pub.IRI(a.Metadata.FollowingIRI)
			}
			if len(a.Metadata.URL) > 0 {
				p.URL = pub.IRI(a.Metadata.URL)
			}
		} else {
			p.Name.Set("en", a.Handle)

			p.Outbox = pub.IRI(BuildCollectionID(a, handlers.Outbox))
			p.Inbox = pub.IRI(BuildCollectionID(a, handlers.Inbox))
			p.Liked = pub.IRI(BuildCollectionID(a, handlers.Liked))

			p.URL = accountURL(a)

			if !a.CreatedAt.IsZero() {
				p.Published = a.CreatedAt
			}
			if !a.UpdatedAt.IsZero() {
				p.Updated = a.UpdatedAt
			}
		}
		if len(a.Hash) >= 8 {
			p.ID = apAccountID(a)
		}
		oauthURL := strings.Replace(BaseURL, "api", "oauth", 1)
		p.Endpoints = &pub.Endpoints{
			SharedInbox:                pub.IRI(fmt.Sprintf("%s/inbox", BaseURL)),
			OauthAuthorizationEndpoint: pub.IRI(fmt.Sprintf("%s/authorize", oauthURL)),
			OauthTokenEndpoint:         pub.IRI(fmt.Sprintf("%s/token", oauthURL)),
		}
	}

	//p.Score = a.Score
	if a.IsValid() && a.HasMetadata() && a.Metadata.Key != nil && a.Metadata.Key.Public != nil {
		p.PublicKey = pub.PublicKey{
			ID:           pub.ID(fmt.Sprintf("%s#main-key", p.ID)),
			Owner:        pub.IRI(p.ID),
			PublicKeyPem: fmt.Sprintf("-----BEGIN PUBLIC KEY-----\n%s\n-----END PUBLIC KEY-----", base64.StdEncoding.EncodeToString(a.Metadata.Key.Public)),
		}
	}
	return &p
}

func getSigner(pubKeyID pub.ID, key crypto.PrivateKey) *httpsig.Signer {
	hdrs := []string{"(request-target)", "host", "date"}
	return httpsig.NewSigner(string(pubKeyID), key, httpsig.RSASHA256, hdrs)
}

func (r *repository) WithAccount(a *Account) error {
	r.fedbox.SignFn(func(req *http.Request) error {
		// TODO(marius): this needs to be added to the federated requests, which we currently don't support
		if !a.IsValid() || !a.IsLogged() {
			return nil
		}
		if a.Metadata.OAuth.Token == "" {
			e := errors.Newf("account has no OAuth2 token")
			r.errFn(e.Error(), log.Ctx{
				"handle":   a.Handle,
				"logged":   a.IsLogged(),
				"metadata": a.Metadata,
			})
			return e
		}
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", a.Metadata.OAuth.Token))
		return nil
	})
	return nil
}

func (r *repository) withAccountS2S(a *Account) error {
	// TODO(marius): this needs to be added to the federated requests, which we currently don't support
	if !a.IsValid() || !a.IsLogged() {
		return nil
	}

	k := a.Metadata.Key
	if k == nil {
		return nil
	}
	var prv crypto.PrivateKey
	var err error
	if k.ID == "id-rsa" {
		prv, err = x509.ParsePKCS8PrivateKey(k.Private)
	}
	if err != nil {
		return err
	}
	if k.ID == "id-ecdsa" {
		return errors.Errorf("unsupported private key type %s", k.ID)
		//prv, err = x509.ParseECPrivateKey(k.Private)
	}
	if err != nil {
		return err
	}
	p := *loadAPPerson(*a)
	s := getSigner(p.PublicKey.ID, prv)
	r.fedbox.SignFn(s.Sign)

	return nil
}

func (r *repository) LoadItem(f Filters) (Item, error) {
	var item Item

	f.MaxItems = 1
	hashes := f.LoadItemsFilter.Key
	f.LoadItemsFilter.Key = nil

	url := fmt.Sprintf("%s/objects/%s", r.BaseURL, hashes[0])
	art, err := r.fedbox.Object(pub.IRI(url))
	if err != nil {
		r.errFn(err.Error(), nil)
		return item, err
	}
	err = item.FromActivityPub(art)
	if err == nil {
		var items ItemCollection
		items, err = r.loadItemsAuthors(item)
		items, err = r.loadItemsVotes(items...)
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

func (r *repository) loadAccountsVotes(accounts ...Account) (AccountCollection, error) {
	if len(accounts) == 0 {
		return accounts, nil
	}
	fVotes := Filters{}
	for _, account := range accounts {
		fVotes.LoadVotesFilter.AttributedTo = append(fVotes.LoadVotesFilter.AttributedTo, account.Hash)
	}
	fVotes.LoadVotesFilter.AttributedTo = hashesUnique(fVotes.LoadVotesFilter.AttributedTo)
	col := make(AccountCollection, len(accounts))
	votes, _, err := r.LoadVotes(fVotes)
	if err != nil {
		return accounts, errors.Annotatef(err, "unable to load accounts votes")
	}
	for k, ac := range accounts {
		for _, vot := range votes {
			if bytes.Equal(vot.SubmittedBy.Hash, ac.Hash) {
				ac.Score += vot.Weight
			}
		}
		col[k] = ac
	}
	return col, nil
}

func accountInCollection(ac Account, col AccountCollection) bool {
	for _, fol := range col {
		if HashesEqual(fol.Hash, ac.Hash) {
			return true
		}
	}
	return false
}

func (r *repository) loadAccountsFollowers(acc Account) (Account, error) {
	if !acc.HasMetadata() || len(acc.Metadata.FollowersIRI) == 0 {
		return acc, nil
	}
	it, err := r.fedbox.Collection(pub.IRI(acc.Metadata.FollowersIRI))
	if err != nil {
		r.errFn(err.Error(), nil)
		return acc, nil
	}
	if !pub.CollectionTypes.Contains(it.GetType()) {
		return acc, nil
	}
	pub.OnOrderedCollection(it, func(o *pub.OrderedCollection) error {
		for _, fol := range o.Collection() {
			if !pub.ActorTypes.Contains(fol.GetType()) {
				continue
			}
			p := Account{}
			p.FromActivityPub(fol)
			if p.IsValid() {
				acc.Followers = append(acc.Followers, p)
			}
		}
		return nil
	})

	return acc, nil
}

func (r *repository) loadAccountsFollowing(acc Account) (Account, error) {
	if !acc.HasMetadata() || len(acc.Metadata.FollowersIRI) == 0 {
		return acc, nil
	}
	it, err := r.fedbox.Collection(pub.IRI(acc.Metadata.FollowingIRI))
	if err != nil {
		r.errFn(err.Error(), nil)
		return acc, nil
	}
	if !pub.CollectionTypes.Contains(it.GetType()) {
		return acc, nil
	}
	pub.OnOrderedCollection(it, func(o *pub.OrderedCollection) error {
		for _, fol := range o.Collection() {
			if !pub.ActorTypes.Contains(fol.GetType()) {
				continue
			}
			p := Account{}
			p.FromActivityPub(fol)
			if p.IsValid() {
				acc.Following = append(acc.Following, p)
			}
		}
		return nil
	})

	return acc, nil
}

func getRepliesOf(items ...Item) pub.IRIs {
	repliesTo := make(pub.IRIs, 0)
	iriFn := func(it Item) pub.IRI {
		if it.pub != nil {
			return it.pub.GetLink()
		}
		if id, ok := BuildIDFromItem(it); ok {
			return pub.IRI(id)
		}
		return ""
	}
	for _, it := range items {
		iri := iriFn(it)
		if len(iri) > 0 && !repliesTo.Contains(iri) {
			repliesTo = append(repliesTo, iri)
		}
		if it.OP.IsValid() {
			iri := iriFn(*it.OP)
			if len(iri) > 0 && !repliesTo.Contains(iri) {
				repliesTo = append(repliesTo, iri)
			}
		}
	}
	return repliesTo
}

func (r *repository) loadItemsReplies(items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return nil, nil
	}
	repliesTo := getRepliesOf(items...)
	if len(repliesTo) == 0 {
		return nil, nil
	}
	allReplies := make(ItemCollection, 0)
	f := &ActivityFilters{}
	for _, top := range repliesTo {
		collFn := func() (pub.CollectionInterface, error) {
			return r.fedbox.Replies(top.GetLink(), Values(f))
		}
		err := LoadFromCollection(collFn, &colCursor{filters: f}, func(c pub.ItemCollection) (bool, error) {
			for _, it := range c {
				if !it.IsObject() {
					continue
				}
				i := Item{}
				i.FromActivityPub(it)
				if !allReplies.Contains(i) {
					allReplies = append(allReplies, i)
				}
			}
			return true, nil
		})
		if err != nil {
			r.errFn(err.Error(), nil)
		}
	}
	// TODO(marius): probably we can thread the replies right here
	return allReplies, nil
}

func (r *repository) loadAccountVotes(acc *Account, items ItemCollection) error {
	if acc == nil || acc.pub == nil {
		return nil
	}
	voteActivities := pub.ActivityVocabularyTypes{pub.LikeType, pub.DislikeType, pub.UndoType}
	f := &ActivityFilters{
		Object: &ActivityFilters{},
		Type:   ActivityTypesFilter(voteActivities...),
	}
	for _, it := range items {
		f.Object.IRI = append(f.Object.IRI, LikeString(it.Hash.String()))
	}
	collFn := func() (pub.CollectionInterface, error) {
		return r.fedbox.Outbox(acc.pub, Values(f))
	}
	return LoadFromCollection(collFn, &colCursor{filters: f}, func(col pub.ItemCollection) (bool, error) {
		for _, it := range col {
			if !it.IsObject() || !voteActivities.Contains(it.GetType()) {
				continue
			}
			v := Vote{}
			v.FromActivityPub(it)
			if !acc.Votes.Contains(v) {
				acc.Votes = append(acc.Votes, v)
			}
		}
		return false, nil
	})
}

func (r *repository) loadItemsVotes(items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}
	voteActivities := pub.ActivityVocabularyTypes{pub.LikeType, pub.DislikeType, pub.UndoType}
	f := &ActivityFilters{
		Object: &ActivityFilters{},
		Type:   ActivityTypesFilter(voteActivities...),
	}
	for _, it := range items {
		f.Object.IRI = append(f.Object.IRI, LikeString(it.Hash.String()))
	}
	collFn := func() (pub.CollectionInterface, error) {
		return r.fedbox.Inbox(r.fedbox.Service(), Values(f))
	}
	err := LoadFromCollection(collFn, &colCursor{filters: f}, func(c pub.ItemCollection) (bool, error) {
		for _, vAct := range c {
			if !vAct.IsObject() || !voteActivities.Contains(vAct.GetType()) {
				continue
			}
			v := Vote{}
			v.FromActivityPub(vAct)
			for k, ob := range items {
				if bytes.Equal(v.Item.Hash, ob.Hash) {
					items[k].Score += v.Weight
				}
			}
		}
		return true, nil
	})
	return items, err
}

func EqualsString(s string) CompStr {
	return CompStr{Operator: "=", Str: s}
}

func ActivityTypesFilter(t ...pub.ActivityVocabularyType) CompStrs {
	r := make(CompStrs, len(t))
	for i, typ := range t {
		r[i] = EqualsString(string(typ))
	}
	return r
}

func (r *repository) loadAuthors(items ...FollowRequest) ([]FollowRequest, error) {
	if len(items) == 0 {
		return items, nil
	}
	fActors := ActivityFilters{
		Type: ActivityTypesFilter(ValidActorTypes...),
	}
	for _, it := range items {
		if !it.SubmittedBy.IsValid() {
			continue
		}
		var hash CompStr
		if it.SubmittedBy.HasMetadata() && len(it.SubmittedBy.Metadata.ID) > 0 {
			hash = EqualsString(it.SubmittedBy.Metadata.ID)
		} else {
			hash = EqualsString(it.SubmittedBy.Hash.String())
		}
		if len(hash.Str) > 0 && !fActors.IRI.Contains(hash) {
			fActors.IRI = append(fActors.IRI, hash)
		}
	}

	if len(fActors.IRI) == 0 {
		return items, errors.Errorf("unable to load items authors")
	}
	authors, err := r.accounts(&fActors)
	if err != nil {
		return items, errors.Annotatef(err, "unable to load items authors")
	}
	for k, it := range items {
		for i, auth := range authors {
			if accountsEqual(*it.SubmittedBy, auth) {
				items[k].SubmittedBy = &(authors[i])
			}
		}
	}
	return items, nil
}

func accountsEqual(a1, a2 Account) bool {
	return bytes.Equal(a1.Hash, a2.Hash) || (len(a1.Handle)+len(a2.Handle) > 0 && a1.Handle == a2.Handle)
}

func (r *repository) loadItemsAuthors(items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}

	fActors := ActivityFilters{
		Type: ActivityTypesFilter(ValidActorTypes...),
	}
	for _, it := range items {
		if it.HasMetadata() {
			// Adding an item's recipients list (To and CC) to the list of accounts we want to load from the ActivityPub API
			if len(it.Metadata.To) > 0 {
				for _, to := range it.Metadata.To {
					hash := EqualsString(to.Metadata.ID)
					if len(hash.Str) > 0 && !fActors.IRI.Contains(hash) {
						fActors.IRI = append(fActors.IRI, hash)
					}
				}
			}
			if len(it.Metadata.CC) > 0 {
				for _, cc := range it.Metadata.CC {
					hash := EqualsString(cc.Metadata.ID)
					if len(hash.Str) > 0 && !fActors.IRI.Contains(hash) {
						fActors.IRI = append(fActors.IRI, hash)
					}
				}
			}
		}
		if !it.SubmittedBy.IsValid() {
			continue
		}
		// Adding an item's author to the list of accounts we want to load from the ActivityPub API
		var hash CompStr
		if it.SubmittedBy.HasMetadata() && len(it.SubmittedBy.Metadata.ID) > 0 {
			hash = EqualsString(it.SubmittedBy.Metadata.ID)
		} else {
			hash = CompStr{Str: it.SubmittedBy.Hash.String()}
		}
		if len(hash.Str) > 0 && !fActors.IRI.Contains(hash) {
			fActors.IRI = append(fActors.IRI, hash)
		}
	}

	if len(fActors.IRI) == 0 {
		return items, errors.Errorf("unable to load items authors")
	}
	col := make(ItemCollection, len(items))
	authors, err := r.accounts(&fActors)
	if err != nil {
		return items, errors.Annotatef(err, "unable to load items authors")
	}
	for k, it := range items {
		for a, auth := range authors {
			if accountsEqual(*it.SubmittedBy, auth) {
				it.SubmittedBy = &(authors[a])
			}
			if it.UpdatedBy.IsValid() && accountsEqual(*it.UpdatedBy, auth) {
				it.UpdatedBy = &(authors[a])
			}
			if !it.HasMetadata() {
				continue
			}
			for i, to := range it.Metadata.To {
				if accountsEqual(*to, auth) {
					it.Metadata.To[i] = &(authors[a])
				}
			}
			for i, cc := range it.Metadata.CC {
				if accountsEqual(*cc, auth) {
					it.Metadata.CC[i] = &(authors[a])
				}
			}
		}
		col[k] = it
	}
	return col, nil
}

type Cursor struct {
	after  Hash
	before Hash
	items  RenderableList
	total  uint
}

var emptyCursor = Cursor{}

type colCursor struct {
	filters *ActivityFilters
	items   pub.ItemCollection
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
	done bool
	err  error
}

// LoadFromCollection iterates over a collection returned by the f function, until accum is satisfied
func LoadFromCollection(f CollectionFn, cur *colCursor, accum func(pub.ItemCollection) (bool, error)) error {
	fetch := make(chan res)

	var err error
	for processed := 0; ; {
		go func() {
			var status bool
			var col pub.CollectionInterface

			col, err = f()
			if err != nil {
				fetch <- res{true, err}
				return
			}

			var prev string
			prev, cur.filters.Next = getCollectionPrevNext(col)
			if processed == 0 {
				cur.filters.Prev = prev
			}
			if col.GetType() == pub.OrderedCollectionPageType {
				pub.OnOrderedCollectionPage(col, func(c *pub.OrderedCollectionPage) error {
					status, err = accum(c.OrderedItems)
					return nil
				})
			}
			if col.GetType() == pub.OrderedCollectionType {
				pub.OnOrderedCollection(col, func(c *pub.OrderedCollection) error {
					status, err = accum(c.OrderedItems)
					return nil
				})
			}
			if col.GetType() == pub.CollectionPageType {
				pub.OnCollectionPage(col, func(c *pub.CollectionPage) error {
					status, err = accum(c.Items)
					return nil
				})
			}
			if col.GetType() == pub.CollectionType {
				pub.OnCollection(col, func(c *pub.Collection) error {
					status, err = accum(c.Items)
					return nil
				})
			}
			if err != nil {
				status = true
			} else {
				processed += len(col.Collection())
			}
			fetch <- res{status || uint(processed) == col.Count(), err}
		}()

		r := <-fetch
		if r.done || len(cur.filters.Next)+len(cur.filters.Prev) == 0 {
			break
		}
	}

	return err
}

func (r *repository) accounts(f *ActivityFilters) ([]Account, error) {
	actors := func() (pub.CollectionInterface, error) {
		return r.fedbox.Actors(Values(f))
	}
	accounts := make([]Account, 0)
	err := LoadFromCollection(actors, &colCursor{filters: f}, func(col pub.ItemCollection) (bool, error) {
		for _, it := range col {
			if !it.IsObject() || !ValidActorTypes.Contains(it.GetType()) {
				continue
			}
			a := Account{}
			a.FromActivityPub(it)
			accounts = append(accounts, a)
		}
		// TODO(marius): this needs to be externalized also to a different function that we can pass from outer scope
		//   This function implements the logic for breaking out of the collection iteration cycle and returns a bool
		return len(accounts) == f.MaxItems || len(f.Next) == 0, nil
	})

	return accounts, err
}

func (r *repository) objects(f *ActivityFilters) (ItemCollection, error) {
	objects := func() (pub.CollectionInterface, error) {
		return r.fedbox.Objects(Values(f))
	}
	items := make(ItemCollection, 0)
	err := LoadFromCollection(objects, &colCursor{filters: f}, func(c pub.ItemCollection) (bool, error) {
		for _, it := range c {
			i := Item{}
			i.FromActivityPub(it)
			items = append(items, i)
		}
		return len(items) == f.MaxItems || len(f.Next) == 0, nil
	})
	return items, err
}

func (r *repository) Objects(f *ActivityFilters) (Cursor, error) {
	items, err := r.objects(f)
	if err != nil {
		return emptyCursor, err
	}
	result := make([]Renderable, 0)
	for _, it := range items {
		if len(it.Hash) > 0 {
			result = append(result, &it)
		}
	}

	return Cursor{
		after:  Hash(f.Next),
		before: Hash(f.Prev),
		items:  result,
		total:  uint(len(result)),
	}, nil
}

func validFederated(i Item, f *ActivityFilters) bool {
	if len(f.Generator) > 0 {
		for _, g := range f.Generator {
			if i.pub == nil || i.pub.Generator == nil {
				continue
			}
			if g == nilIRI {
				if i.pub.Generator.GetLink().Equals(pub.IRI(Instance.BaseURL), false) {
					return false
				}
				return true
			}
			if i.pub.Generator.GetLink().Equals(pub.IRI(g.Str), false) {
				return true
			}
		}
	}
	// @todo(marius): currently this marks as valid nil generator, but we eventually want non nil generators
	return i.pub != nil && i.pub.Generator == nil
}

func validRecipients(i Item, f *ActivityFilters) bool {
	if len(f.Recipients) > 0 {
		for _, r := range f.Recipients {
			if pub.IRI(r.Str).Equals(pub.PublicNS, false) && i.Private() {
				return false
			}
		}
	}
	return true
}

func validItem(it Item, f *ActivityFilters) bool {
	if keep := validRecipients(it, f); !keep {
		return keep
	}
	if keep := validFederated(it, f); !keep {
		return keep
	}
	return true
}

func filterItems(items ItemCollection, f *ActivityFilters) ItemCollection {
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

func orderRenderables(r RenderableList) {
	sort.SliceStable(r, func(i, j int) bool {
		return r[i].Date().After(r[j].Date())
	})
}

// ActorCollection loads the service's collection returned by fn.
// First step is to load the Create activities from the inbox
// Iterating over the activities in the resulting collection, we gather the objects and accounts
//  With the resulting Object IRIs we load from the objects collection with our matching filters
//  With the resulting Actor IRIs we load from the accounts collection with matching filters
// From the
func (r *repository) ActorCollection(fn CollectionFn, f *ActivityFilters) (Cursor, error) {
	items := make(ItemCollection, 0)
	follows := make(FollowRequests, 0)
	relations := make(map[pub.IRI]pub.IRI)
	err := LoadFromCollection(fn, &colCursor{filters: f}, func(col pub.ItemCollection) (bool, error) {
		deferredItems := make(CompStrs, 0)
		for _, it := range col {
			pub.OnActivity(it, func(a *pub.Activity) error {
				if it.GetType() == pub.CreateType {
					ob := a.Object
					i := Item{}
					if ob.IsObject() {
						if ValidItemTypes.Contains(ob.GetType()) {
							i.FromActivityPub(ob)
							if validItem(i, f) {
								items = append(items, i)
							}
						}
					} else {
						i.FromActivityPub(a)
						uuid := EqualsString(path.Base(ob.GetLink().String()))
						if !deferredItems.Contains(uuid) && validItem(i, f) {
							deferredItems = append(deferredItems, uuid)
						}
					}
					relations[a.GetLink()] = ob.GetLink()
				}
				if it.GetType() == pub.FollowType {
					// @todo(marius)
					f := FollowRequest{}
					f.FromActivityPub(a)
					follows = append(follows, f)
					relations[a.GetLink()] = a.GetLink()
				}
				return nil
			})
		}

		if len(deferredItems) > 0 {
			ff := f.Object
			ff.IRI = deferredItems
			ff.MaxItems = f.MaxItems - len(items)
			objects, err := r.objects(ff)
			if err != nil {
				return true, err
			}
			for _, d := range objects {
				if !items.Contains(d) {
					items = append(items, d)
				}
			}
		}

		// TODO(marius): this needs to be externalized also to a different function that we can pass from outer scope
		//   This function implements the logic for breaking out of the collection iteration cycle and returns a bool
		return len(items) >= f.MaxItems || len(f.Next) == 0, nil
	})
	if err != nil {
		return emptyCursor, err
	}
	items, err = r.loadItemsAuthors(items...)
	if err != nil {
		return emptyCursor, err
	}
	items, err = r.loadItemsVotes(items...)
	if err != nil {
		return emptyCursor, err
	}
	//items, err = r.loadItemsReplies(items...)
	//if err != nil {
	//	return emptyCursor, err
	//}
	follows, err = r.loadAuthors(follows...)
	if err != nil {
		return emptyCursor, err
	}

	result := make([]Renderable, 0)
	for _, iI := range relations {
		for _, it := range items {
			if it.IsValid() && it.pub.GetLink() == iI {
				result = append(result, &it)
				break
			}
		}
		for _, f := range follows {
			if f.pub != nil && f.pub.GetLink() == iI {
				result = append(result, &f)
				break
			}
		}
	}
	orderRenderables(result)

	return Cursor{
		after:  Hash(f.Next),
		before: Hash(f.Prev),
		items:  result,
		total:  uint(len(result)),
	}, nil
}

func (r *repository) LoadItems(f Filters) (ItemCollection, uint, error) {
	target := "/"
	c := "objects"
	if len(f.FollowedBy) > 0 {
		// TODO(marius): make this work for multiple FollowedBy filters
		target = fmt.Sprintf("/%s/%s/", actors, f.FollowedBy)
		c = "inbox"
		f.FollowedBy = ""
		f.LoadItemsFilter.IRI = ""
		f.Federated = nil
		f.InReplyTo = nil
		f.Type = pub.ActivityVocabularyTypes{
			pub.CreateType,
		}
	}
	filterLocal := false
	keepPrivate := true
	if len(f.Federated) > 0 {
		for _, fed := range f.Federated {
			if !fed {
				// TODO(marius): need to add to fedbox support for filtering by hostname
				f.LoadItemsFilter.IRI = BaseURL
			} else {
				filterLocal = true
			}
			break
		}
	}
	if len(f.Private) > 0 {
		for _, prv := range f.Private {
			if keepPrivate := !prv; keepPrivate {
				break
			}
		}
	}
	if len(f.Type) == 0 && len(f.LoadItemsFilter.Deleted) > 0 {
		f.Type = ValidItemTypes
	}
	url := fmt.Sprintf("%s%s%s", r.BaseURL, target, c)

	ctx := log.Ctx{
		"url": url,
	}

	it, err := r.fedbox.Collection(pub.IRI(url), Values(f))
	if err != nil {
		r.errFn(err.Error(), ctx)
		return nil, 0, err
	}

	items := make(ItemCollection, 0)
	var count uint = 0
	pub.OnOrderedCollection(it, func(col *pub.OrderedCollection) error {
		count = col.TotalItems
		for _, it := range col.OrderedItems {
			if filterLocal && it.GetLink().Contains(pub.IRI(r.BaseURL), false) {
				continue
			}
			if !filterLocal && !it.GetLink().Contains(pub.IRI(r.BaseURL), false) {
				continue
			}
			i := Item{}
			if err := i.FromActivityPub(it); err != nil {
				r.errFn(err.Error(), nil)
				continue
			}
			if keepPrivate && i.Private() {
				continue
			}
			items = append(items, i)
		}
		return nil
	})

	// TODO(marius): move this somewhere more palatable
	//  it's currently done like this when loading from collections of Activities that only contain the ID of the object
	toLoad := make(Hashes, 0)
	for _, i := range items {
		if i.IsValid() && !i.Deleted() && i.SubmittedAt.IsZero() {
			toLoad = append(toLoad, Hash(i.Metadata.ID))
		}
	}
	if len(toLoad) > 0 {
		return r.LoadItems(Filters{
			LoadItemsFilter: LoadItemsFilter{
				Key: toLoad,
			},
		})
	}
	items, err = r.loadItemsAuthors(items...)
	if Instance.Config.VotingEnabled {
		items, err = r.loadItemsVotes(items...)
	}

	return items, count, err
}

func (r *repository) SaveVote(v Vote) (Vote, error) {
	if !v.SubmittedBy.IsValid() || !v.SubmittedBy.HasMetadata() {
		return Vote{}, errors.Newf("Invalid vote submitter")
	}
	if !v.Item.IsValid() || !v.Item.HasMetadata() {
		return Vote{}, errors.Newf("Invalid vote item")
	}
	author := loadAPPerson(*v.SubmittedBy)
	if !accountValidForC2S(v.SubmittedBy) {
		return v, errors.Unauthorizedf("invalid account %s", v.SubmittedBy.Handle)
	}

	url := fmt.Sprintf("%s/%s", v.Item.Metadata.ID, "likes")
	itemVotes, err := r.loadVotesCollection(pub.IRI(url), pub.IRI(v.SubmittedBy.Metadata.ID))
	// first step is to verify if vote already exists:
	if err != nil {
		r.errFn(err.Error(), log.Ctx{
			"url": url,
			"err": err,
		})
	}
	var exists Vote
	for _, vot := range itemVotes {
		if !vot.SubmittedBy.IsValid() || !v.SubmittedBy.IsValid() {
			continue
		}
		if bytes.Equal(vot.SubmittedBy.Hash, v.SubmittedBy.Hash) {
			exists = vot
			break
		}
	}

	o := loadAPItem(*v.Item)
	act := pub.Activity{
		Type:  pub.UndoType,
		To:    pub.ItemCollection{pub.PublicNS},
		BCC:   pub.ItemCollection{pub.IRI(BaseURL)},
		Actor: author.GetLink(),
	}

	if exists.HasMetadata() {
		act.Object = pub.IRI(exists.Metadata.IRI)
		if _, _, err := r.fedbox.ToOutbox(act); err != nil {
			r.errFn(err.Error(), nil)
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

	_, _, err = r.fedbox.ToOutbox(act)
	if err != nil {
		r.errFn(err.Error(), nil)
		return v, err
	}
	err = v.FromActivityPub(act)
	return v, err
}

func (r *repository) loadVotesCollection(iri pub.IRI, actors ...pub.IRI) ([]Vote, error) {
	cntActors := len(actors)
	f := Filters{}
	if cntActors > 0 {
		attrTo := make([]Hash, cntActors)
		for i, a := range actors {
			attrTo[i] = Hash(a.String())
		}
		f.LoadVotesFilter.AttributedTo = attrTo
	}
	likes, err := r.fedbox.Collection(iri, Values(f))
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

func (r *repository) LoadVotes(f Filters) (VoteCollection, uint, error) {
	f.Type = pub.ActivityVocabularyTypes{
		pub.LikeType,
		pub.DislikeType,
		pub.UndoType,
	}

	var url string
	if len(f.LoadVotesFilter.AttributedTo) == 1 {
		attrTo := f.LoadVotesFilter.AttributedTo[0]
		f.LoadVotesFilter.AttributedTo = nil
		url = fmt.Sprintf("%s/%s/%s/outbox", r.BaseURL, actors, attrTo)
	} else {
		url = fmt.Sprintf("%s/inbox", r.BaseURL)
	}

	it, err := r.fedbox.Collection(pub.IRI(url), Values(f))
	if err != nil {
		r.errFn(err.Error(), nil)
		return nil, 0, err
	}
	var count uint = 0
	votes := make(VoteCollection, 0)
	undos := make(VoteCollection, 0)
	pub.OnOrderedCollection(it, func(col *pub.OrderedCollection) error {
		count = col.TotalItems
		for _, it := range col.OrderedItems {
			vot := Vote{}
			if err := vot.FromActivityPub(it); err != nil {
				r.errFn(err.Error(), log.Ctx{
					"type": fmt.Sprintf("%T", it),
				})
				continue
			}
			if vot.Weight != 0 {
				votes = append(votes, vot)
			} else {
				if vot.Metadata == nil || len(vot.Metadata.OriginalIRI) == 0 {
					r.infoFn("Zero vote without an original activity undone", nil)
					continue
				}
				undos = append(undos, vot)
			}
		}
		for _, undo := range undos {
			if undo.Metadata == nil || len(undo.Metadata.OriginalIRI) == 0 {
				// error
				continue
			}
			for i, vot := range votes {
				if vot.Metadata == nil || len(vot.Metadata.IRI) == 0 {
					// error
					continue
				}
				if vot.Metadata.IRI == undo.Metadata.OriginalIRI {
					votes = append(votes[:i], votes[i+1:]...)
				}
			}
		}
		return err
	})

	return votes, count, nil
}

func (r *repository) LoadVote(f Filters) (Vote, error) {
	if len(f.ItemKey) == 0 {
		return Vote{}, errors.Newf("invalid item hash")
	}

	v := Vote{}
	itemHash := f.ItemKey[0]
	f.ItemKey = nil
	url := fmt.Sprintf("%s/liked/%s", r.BaseURL, itemHash)

	like, err := r.fedbox.Activity(pub.IRI(url))
	if err != nil {
		r.errFn(err.Error(), nil)
		return v, err
	}
	err = v.FromActivityPub(like)
	return v, err
}

type _errors struct {
	Ctxt   string        `jsonld:"@context"`
	Errors []errors.Http `jsonld:"errors"`
}

func (r *repository) handlerErrorResponse(body []byte) error {
	errs := _errors{}
	if err := j.Unmarshal(body, &errs); err != nil {
		r.errFn(fmt.Sprintf("Unable to unmarshal error response: %s", err.Error()), nil)
		return nil
	}
	if len(errs.Errors) == 0 {
		return nil
	}
	err := errs.Errors[0]
	return errors.WrapWithStatus(err.Code, nil, err.Message)
}

func (r *repository) handleItemSaveSuccessResponse(it Item, body []byte) (Item, error) {
	ap, err := pub.UnmarshalJSON(body)
	if err != nil {
		r.errFn(err.Error(), nil)
		return it, err
	}
	err = it.FromActivityPub(ap)
	if err != nil {
		r.errFn(err.Error(), nil)
		return it, err
	}
	items, err := r.loadItemsAuthors(it)
	return items[0], err
}

func accountValidForC2S(a *Account) bool {
	return a.IsValid() && a.IsLogged()
}

func (r *repository) getAuthorRequestURL(a *Account) string {
	var reqURL string
	if a.IsValid() && a.IsLogged() {
		author := loadAPPerson(*a)
		if a.IsLocal() {
			reqURL = author.Outbox.GetLink().String()
		} else {
			reqURL = author.Inbox.GetLink().String()
		}
	} else {
		author := anonymousPerson(r.BaseURL)
		reqURL = author.Inbox.GetLink().String()
	}
	return reqURL
}

func (r *repository) SaveItem(it Item) (Item, error) {
	if !it.SubmittedBy.IsValid() || !it.SubmittedBy.HasMetadata() {
		return Item{}, errors.Newf("Invalid item submitter")
	}
	art := loadAPItem(it)
	author := loadAPPerson(*it.SubmittedBy)
	if !accountValidForC2S(it.SubmittedBy) {
		return it, errors.Unauthorizedf("invalid account %s", it.SubmittedBy.Handle)
	}

	to := make(pub.ItemCollection, 0)
	cc := make(pub.ItemCollection, 0)
	bcc := make(pub.ItemCollection, 0)

	var err error
	id := art.GetLink()

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
		if len(it.Metadata.Mentions) > 0 {
			names := make([]string, 0)
			for _, m := range it.Metadata.Mentions {
				names = append(names, m.Name)
			}
			ff := Filters{
				LoadAccountsFilter: LoadAccountsFilter{
					Handle: names,
				},
			}
			actors, _, err := r.LoadAccounts(ff)
			if err != nil {
				r.errFn("unable to load accounts from mentions", log.Ctx{"err": err})
			}
			for _, actor := range actors {
				if actor.HasMetadata() && len(actor.Metadata.ID) > 0 {
					cc = append(cc, pub.IRI(actor.Metadata.ID))
				}
			}
		}
	}

	if !it.Private() {
		to = append(to, pub.PublicNS)
		if it.Parent == nil && it.SubmittedBy.HasMetadata() && len(it.SubmittedBy.Metadata.FollowersIRI) > 0 {
			cc = append(cc, pub.IRI(it.SubmittedBy.Metadata.FollowersIRI))
		}
		bcc = append(bcc, pub.IRI(BaseURL))
	}

	act := &pub.Activity{
		To:     to,
		CC:     cc,
		BCC:    bcc,
		Actor:  author.GetLink(),
		Object: art,
	}
	if it.Deleted() {
		if len(id) == 0 {
			r.errFn(err.Error(), log.Ctx{
				"item": it.Hash,
			})
			return it, errors.NotFoundf("item hash is empty, can not delete")
		}
		act.Object = id
		act.Type = pub.DeleteType
	} else {
		if len(id) == 0 {
			act.Type = pub.CreateType
		} else {
			act.Type = pub.UpdateType
		}
	}
	_, ob, err := r.fedbox.ToOutbox(act)
	if err != nil {
		r.errFn(err.Error(), nil)
		return it, err
	}
	err = it.FromActivityPub(ob)
	if err != nil {
		r.errFn(err.Error(), nil)
		return it, err
	}
	items, err := r.loadItemsAuthors(it)
	return items[0], err
}

func (r *repository) LoadAccounts(f Filters) (AccountCollection, uint, error) {
	it, err := r.fedbox.Actors(Values(&(f.LoadAccountsFilter)))
	if err != nil {
		r.errFn(err.Error(), nil)
		return nil, 0, err
	}
	accounts := make(AccountCollection, 0)
	var count uint = 0
	pub.OnOrderedCollection(it, func(col *pub.OrderedCollection) error {
		count = col.TotalItems
		for _, it := range col.OrderedItems {
			acc := Account{Metadata: &AccountMetadata{}}
			if err := acc.FromActivityPub(it); err != nil {
				r.errFn(err.Error(), log.Ctx{
					"type": fmt.Sprintf("%T", it),
				})
				continue
			}
			accounts = append(accounts, acc)
		}
		accounts, err = r.loadAccountsVotes(accounts...)
		return err
	})
	return accounts, count, nil
}

func (r *repository) LoadAccount(f Filters) (Account, error) {
	var accounts AccountCollection
	var err error
	if accounts, _, err = r.LoadAccounts(f); err != nil {
		return AnonymousAccount, err
	}

	var ac *Account
	if ac, err = accounts.First(); err != nil {
		var id string
		if len(f.LoadAccountsFilter.Key) > 0 {
			id = f.LoadAccountsFilter.Key[0].String()
		}
		if len(f.Handle) > 0 {
			id = f.Handle[0]
		}
		return AnonymousAccount, errors.NotFoundf("account %s", id)
	}
	return *ac, nil
}

func Values(f interface{}) func() url.Values {
	return func() url.Values {
		v, e := qstring.Marshal(f)
		if e != nil {
			return url.Values{}
		}
		return v
	}
}

func (r *repository) LoadFollowRequests(ed *Account, f Filters) (FollowRequests, uint, error) {
	if len(f.Type) == 0 {
		f.Type = pub.ActivityVocabularyTypes{pub.FollowType}
	}
	var followReq pub.CollectionInterface
	var err error
	if ed == nil {
		followReq, err = r.fedbox.Activities(Values(f))
	} else {
		followReq, err = r.fedbox.Inbox(loadAPPerson(*ed), Values(f))
	}
	requests := make([]FollowRequest, 0)
	if err == nil && len(followReq.Collection()) > 0 {
		for _, fr := range followReq.Collection() {
			f := FollowRequest{}
			if err := f.FromActivityPub(fr); err == nil {
				if !accountInCollection(*f.SubmittedBy, ed.Followers) {
					requests = append(requests, f)
				}
			}
		}
		requests, err = r.loadAuthors(requests...)
	}
	return requests, uint(len(requests)), nil
}

func (r *repository) SendFollowResponse(f FollowRequest, accept bool) error {
	ed := f.Object
	er := f.SubmittedBy
	if !accountValidForC2S(ed) {
		return errors.Unauthorizedf("invalid account %s", ed.Handle)
	}

	to := make(pub.ItemCollection, 0)
	bcc := make(pub.ItemCollection, 0)

	to = append(to, pub.IRI(er.Metadata.ID))
	bcc = append(bcc, pub.IRI(BaseURL))

	response := pub.Activity{
		To:     to,
		Type:   pub.RejectType,
		BCC:    bcc,
		Object: pub.IRI(f.Metadata.ID),
		Actor:  pub.IRI(ed.Metadata.ID),
	}
	if accept {
		to = append(to, pub.PublicNS)
		response.Type = pub.AcceptType
	}

	_, _, err := r.fedbox.ToOutbox(response)
	if err != nil {
		r.errFn(err.Error(), nil)
		return err
	}
	return nil
}

func (r *repository) FollowAccount(er, ed Account) error {
	follower := loadAPPerson(er)
	followed := loadAPPerson(ed)
	if !accountValidForC2S(&er) {
		return errors.Unauthorizedf("invalid account %s", er.Handle)
	}

	to := make(pub.ItemCollection, 0)
	bcc := make(pub.ItemCollection, 0)

	//to = append(to, follower.GetLink())
	to = append(to, pub.PublicNS)
	bcc = append(bcc, pub.IRI(BaseURL))

	follow := pub.Follow{
		Type:   pub.FollowType,
		To:     to,
		BCC:    bcc,
		Object: followed.GetLink(),
		Actor:  follower.GetLink(),
	}
	_, _, err := r.fedbox.ToOutbox(follow)
	if err != nil {
		r.errFn(err.Error(), nil)
		return err
	}
	return nil
}

func (r *repository) SaveAccount(a Account) (Account, error) {
	p := loadAPPerson(a)
	id := p.GetLink()

	now := time.Now().UTC()

	p.Published = now
	p.Updated = now

	author := loadAPPerson(*a.CreatedBy)
	act := pub.Activity{
		To:           pub.ItemCollection{pub.PublicNS},
		BCC:          pub.ItemCollection{pub.IRI(BaseURL)},
		AttributedTo: author.GetLink(),
		Updated:      now,
		Actor:        author.GetLink(),
	}

	var err error
	if a.Deleted() {
		if len(id) == 0 {
			err := errors.NotFoundf("item hash is empty, can not delete")
			r.infoFn(err.Error(), log.Ctx{
				"account": a.Hash,
			})
			return a, err
		}
		act.Type = pub.DeleteType
		act.Object = id
	} else {
		act.Object = p
		p.To = pub.ItemCollection{pub.PublicNS}
		p.BCC = pub.ItemCollection{pub.IRI(BaseURL)}
		if len(id) == 0 {
			act.Type = pub.CreateType
		} else {
			act.Type = pub.UpdateType
		}
	}

	var ap pub.Item
	if _, ap, err = r.fedbox.ToOutbox(act); err != nil {
		r.errFn(err.Error(), nil)
		return a, err
	}
	err = a.FromActivityPub(ap)
	if err != nil {
		r.errFn(err.Error(), nil)
	}
	return a, err
}

// LoadInfo this method is here to keep compatibility with the repository interfaces
// but in the long term we might want to store some of this information in the DB
func (r *repository) LoadInfo() (WebInfo, error) {
	return Instance.NodeInfo(), nil
}

func (r *repository) LoadActorOutbox(actor pub.Item, f *ActivityFilters) (*Cursor, error) {
	if actor == nil {
		return nil, errors.Errorf("Invalid actor")
	}
	outbox := func() (pub.CollectionInterface, error) {
		return r.fedbox.Outbox(actor, Values(f))
	}
	cursor, err := r.ActorCollection(outbox, f)
	if err != nil {
		return nil, err
	}
	return &cursor, nil
}

func (r *repository) LoadActorInbox(actor pub.Item, f *ActivityFilters) (*Cursor, error) {
	if actor == nil {
		return nil, errors.Errorf("Invalid actor")
	}
	collFn := func() (pub.CollectionInterface, error) {
		return r.fedbox.Inbox(actor, Values(f))
	}
	cursor, err := r.ActorCollection(collFn, f)
	if err != nil {
		return nil, err
	}
	return &cursor, nil
}
