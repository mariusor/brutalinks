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
	"strconv"
	"strings"
	"time"
)

type repository struct {
	BaseURL string
	app     *Account
	fedbox  *fedbox
	infoFn  LogFn
	errFn   LogFn
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
	ActorsURL = fmt.Sprintf("%s/actors", BaseURL)
	ObjectsURL = fmt.Sprintf("%s/objects", BaseURL)

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
var ActorsURL = fmt.Sprintf("%s/actors", BaseURL)
var ObjectsURL = fmt.Sprintf("%s/objects", BaseURL)

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

	//o.Generator = pub.IRI(app.Instance.BaseURL)
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
		bcc = append(bcc, pub.ItemCollection{pub.IRI(BaseURL)})
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

func hashInCollection(h Hash, col AccountCollection) bool {
	for _, fol := range col {
		if HashesEqual(fol.Hash, h) {
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

func (r *repository) loadItemsReplies(items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}

	repliesTo := make([]string, 0)
	for _, it := range items {
		if it.OP.IsValid() {
			if id, ok := BuildIDFromItem(*it.OP); ok {
				repliesTo = append(repliesTo, string(id))
			}
		}
	}

	var err error
	if len(repliesTo) > 0 {
		f := Filters{
			LoadItemsFilter: LoadItemsFilter{
				InReplyTo: repliesTo,
			},
		}
		items, _, err = r.LoadItems(f)
	}

	return items, err
}

func (r *repository) loadItemsVotes(items ...Item) (ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}
	fVotes := Filters{}
	for _, it := range items {
		fVotes.LoadVotesFilter.ItemKey = append(fVotes.LoadVotesFilter.ItemKey, it.Hash)
	}
	fVotes.LoadVotesFilter.ItemKey = hashesUnique(fVotes.LoadVotesFilter.ItemKey)
	col := make(ItemCollection, len(items))
	votes, _, err := r.LoadVotes(fVotes)
	if err != nil {
		return items, errors.Annotatef(err, "unable to load items votes")
	}
	for k, it := range items {
		for _, vot := range votes {
			if bytes.Equal(vot.Item.Hash, it.Hash) {
				it.Score += vot.Weight
			}
		}
		col[k] = it
	}
	return col, nil
}

func (r *repository) loadAuthors(items ...FollowRequest) ([]FollowRequest, error) {
	if len(items) == 0 {
		return items, nil
	}
	fActors := Filters{}
	for _, it := range items {
		if !it.SubmittedBy.IsValid() {
			continue
		}
		fActors.LoadAccountsFilter.Key = append(fActors.LoadAccountsFilter.Key, it.SubmittedBy.Hash)
	}
	fActors.LoadAccountsFilter.Key = hashesUnique(fActors.LoadAccountsFilter.Key)
	if len(fActors.LoadAccountsFilter.Key)+len(fActors.Handle) == 0 {
		return items, errors.Errorf("unable to load items authors")
	}
	authors, _, err := r.LoadAccounts(fActors)
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

	fActors := Filters{}
	for _, it := range items {
		if it.HasMetadata() {
			// Adding an item's recipients list (To and CC) to the list of actors we want to load from the ActivityPub API
			if len(it.Metadata.To) > 0 {
				for _, to := range it.Metadata.To {
					fActors.LoadAccountsFilter.Key = append(fActors.LoadAccountsFilter.Key, Hash(to.Metadata.ID))
				}
			}
			if len(it.Metadata.CC) > 0 {
				for _, cc := range it.Metadata.CC {
					fActors.LoadAccountsFilter.Key = append(fActors.LoadAccountsFilter.Key, Hash(cc.Metadata.ID))
				}
			}
		}
		if !it.SubmittedBy.IsValid() {
			continue
		}
		// Adding an item's author to the list of actors we want to load from the ActivityPub API
		if it.SubmittedBy.HasMetadata() && len(it.SubmittedBy.Metadata.ID) > 0 {
			fActors.LoadAccountsFilter.Key = append(fActors.LoadAccountsFilter.Key, Hash(it.SubmittedBy.Metadata.ID))
		} else if len(it.SubmittedBy.Hash) > 0 {
			fActors.LoadAccountsFilter.Key = append(fActors.LoadAccountsFilter.Key, it.SubmittedBy.Hash)
		} else if len(it.SubmittedBy.Handle) > 0 {
			fActors.Handle = append(fActors.Handle, it.SubmittedBy.Handle)
		}
	}
	fActors.LoadAccountsFilter.Key = hashesUnique(fActors.LoadAccountsFilter.Key)
	if len(fActors.LoadAccountsFilter.Key)+len(fActors.Handle) == 0 {
		return items, errors.Errorf("unable to load items authors")
	}
	col := make(ItemCollection, len(items))
	authors, _, err := r.LoadAccounts(fActors)
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
	next  int
	prev  int
	total uint
}

var emptyCursor = Cursor{}

func (r *repository) Inbox(f *fedFilters) ([]Renderable, Cursor, error) {
	col, err := r.fedbox.Inbox(r.fedbox.Service(), Values(f))
	if err != nil {
		return nil, emptyCursor, err
	}
	itemTypes := pub.ActivityVocabularyTypes{
		pub.ArticleType,
		pub.NoteType,
		pub.LinkType,
		pub.DocumentType,
		pub.VideoType,
		pub.AudioType,
	}

	var cursor Cursor
	items := make([]Renderable, 0)
	var count uint = 0
	toDeref := make(pub.IRIs, 0)

	cursorFromColPage := func(cur *Cursor, p *pub.OrderedCollectionPage) {
		if pURL, err := p.Prev.GetLink().URL(); err == nil {
			if pPage, err := strconv.ParseInt(pURL.Query().Get("page"), 10, 16); err == nil {
				cursor.prev = int(pPage)
			}
		}
		if nURL, err := p.Next.GetLink().URL(); err == nil {
			if nPage, err := strconv.ParseInt(nURL.Query().Get("page"), 10, 16); err == nil {
				cursor.next = int(nPage)
			}
		}
	}
	cursorFromCol := func(cur *Cursor, c *pub.OrderedCollection) {
		cursor.total = count
		if pURL, err := c.First.GetLink().URL(); err == nil {
			if pPage, err := strconv.ParseInt(pURL.Query().Get("page"), 10, 16); err == nil {
				cursor.next = int(pPage) + 1
			}
		}
	}
	itemsFromCol := func(c *pub.OrderedCollection) error {
		count = c.TotalItems
		cursorFromCol(&cursor, c)

		for _, it := range c.OrderedItems {
			pub.OnActivity(it, func(a *pub.Activity) error {
				ob := a.Object
				if ob.IsObject() {
					if itemTypes.Contains(ob.GetType()) {
						i := Item{}
						i.FromActivityPub(ob)
						items = append(items, &comment{Item: i})
					}
				} else {
					toDeref = append(toDeref, ob.GetLink())
				}
				return nil
			})
		}
		return nil
	}

	if col.GetType() == pub.OrderedCollectionPageType {
		pub.OnOrderedCollectionPage(col, func(p *pub.OrderedCollectionPage) error {
			pub.OnOrderedCollection(p, func(c *pub.OrderedCollection) error {
				return itemsFromCol(c)
			})
			cursorFromColPage(&cursor, p)
			return nil
		})
	}
	if col.GetType() == pub.OrderedCollectionType {
		pub.OnOrderedCollection(col, func(c *pub.OrderedCollection) error {
			return itemsFromCol(c)
		})
	}

	if len(toDeref) > 0 {
		f := fedFilters{
			IRI:  toDeref,
			Type: itemTypes,
		}
		objects, err := r.fedbox.Objects(Values(&f))
		if err != nil {
			// err
		}
		pub.OnOrderedCollection(objects, func(c *pub.OrderedCollection) error {
			for _, it := range c.OrderedItems {
				if it.IsObject() {
					if itemTypes.Contains(it.GetType()) {
						i := Item{}
						i.FromActivityPub(it)
						items = append(items, &comment{Item: i})
					}
				}
			}
			return nil
		})
	}

	return items, emptyCursor, nil
}

func (r *repository) LoadItems(f Filters) (ItemCollection, uint, error) {
	target := "/"
	c := "objects"
	if len(f.FollowedBy) > 0 {
		// TODO(marius): make this work for multiple FollowedBy filters
		target = fmt.Sprintf("/actors/%s/", f.FollowedBy)
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
		f.Type = pub.ActivityVocabularyTypes{
			pub.ArticleType,
			pub.AudioType,
			pub.DocumentType,
			pub.ImageType,
			pub.NoteType,
			pub.PageType,
			pub.VideoType,
		}
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
		url = fmt.Sprintf("%s/actors/%s/outbox", r.BaseURL, attrTo)
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
				r.errFn("unable to load actors from mentions", log.Ctx{"err": err})
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
		bcc = append(bcc, pub.ItemCollection{pub.IRI(BaseURL)})
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
	f.Type = pub.ActivityVocabularyTypes {
		pub.PersonType,
	}
	it, err := r.fedbox.Actors(Values(f))
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
		v, _ := qstring.Marshal(f)
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
	bcc = append(bcc, pub.ItemCollection{pub.IRI(BaseURL)})

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

	p.Generator = pub.IRI(Instance.BaseURL)
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

func (r *repository) Load(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var cursor Cursor
		var err error

		m := listingModel{}
		m.User = account(req)
		m.Items, cursor, err = r.Inbox(FiltersFromContext(req.Context()))
		if err != nil {
			// err
		}

		if cursor.next > 0 {
			m.nextPage = cursor.next
		}
		if cursor.prev > 0 {
			m.prevPage = cursor.prev
		}
		ctx := context.WithValue(req.Context(), CollectionCtxtKey, &m)
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}
