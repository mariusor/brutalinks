package frontend

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	pub "github.com/go-ap/activitypub"
	cl "github.com/go-ap/client"
	"github.com/go-ap/errors"
	"github.com/go-ap/handlers"
	j "github.com/go-ap/jsonld"
	local "github.com/mariusor/littr.go/activitypub"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/mariusor/qstring"
	"github.com/spacemonkeygo/httpsig"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type repository struct {
	BaseURL string
	logger  log.Logger
	client  cl.HttpClient
}

// Repository middleware
func (h handler) Repository(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), app.RepositoryCtxtKey, h.storage)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func NewRepository(c Config) *repository {
	pub.ItemTyperFunc = local.JSONGetItemByType
	cl.UserAgent = fmt.Sprintf("%s-%s", app.Instance.HostName, app.Instance.Version)
	cl.ErrorLogger = func(el ...interface{}) { c.Logger.WithContext(log.Ctx{"client": "api"}).Errorf("%v", el) }
	cl.InfoLogger = func(el ...interface{}) { c.Logger.WithContext(log.Ctx{"client": "api"}).Debugf("%v", el) }

	BaseURL = c.APIURL
	ActorsURL = fmt.Sprintf("%s/actors", BaseURL)
	ObjectsURL = fmt.Sprintf("%s/objects", BaseURL)

	return &repository{
		BaseURL: c.APIURL,
		logger:  c.Logger,
		client:  cl.NewClient(),
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

func BuildCollectionID(a app.Account, o handlers.CollectionType) pub.ID {
	if len(a.Handle) > 0 {
		return pub.ID(fmt.Sprintf("%s/%s/%s", ActorsURL, url.PathEscape(a.Hash.String()), o))
	}
	return pub.ID(fmt.Sprintf("%s/%s", BaseURL, o))
}

var BaseURL = "http://fedbox.git"
var ActorsURL = fmt.Sprintf("%s/actors", BaseURL)
var ObjectsURL = fmt.Sprintf("%s/objects", BaseURL)

func apAccountID(a app.Account) pub.ID {
	if len(a.Hash) >= 8 {
		return pub.ID(fmt.Sprintf("%s/%s", ActorsURL, a.Hash.String()))
	}
	return pub.ID(fmt.Sprintf("%s/anonymous", ActorsURL))
}

func accountURL(acc app.Account) pub.IRI {
	return pub.IRI(fmt.Sprintf("%s%s", app.Instance.BaseURL, AccountPermaLink(acc)))
}

func BuildIDFromItem(i app.Item) (pub.ID, bool) {
	if !i.IsValid() {
		return "", false
	}
	if i.HasMetadata() && len(i.Metadata.ID) > 0 {
		return pub.ID(i.Metadata.ID), true
	}
	return pub.ID(fmt.Sprintf("%s/%s", ObjectsURL, url.PathEscape(i.Hash.String()))), true
}

func BuildActorID(a app.Account) pub.ID {
	if !a.IsValid() {
		return pub.ID(pub.PublicNS)
	}
	if a.HasMetadata() && len(a.Metadata.ID) > 0 {
		return pub.ID(a.Metadata.ID)
	}
	return pub.ID(fmt.Sprintf("%s/%s", ActorsURL, url.PathEscape(a.Hash.String())))
}

func loadAPItem(item app.Item) pub.Item {
	o := local.Object{}

	if id, ok := BuildIDFromItem(item); ok {
		o.ID = id
	}
	if item.MimeType == app.MimeTypeURL {
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
		case app.MimeTypeMarkdown:
			o.Object.Source.MediaType = pub.MimeType(item.MimeType)
			o.MediaType = pub.MimeType(app.MimeTypeHTML)
			if item.Data != "" {
				o.Source.Content.Set("en", item.Data)
				o.Content.Set("en", string(app.Markdown(item.Data)))
			}
		case app.MimeTypeText:
			fallthrough
		case app.MimeTypeHTML:
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
	o.Score = item.Score / app.ScoreMultiplier
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
		{pub.NilLangRef, app.Anonymous},
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

func loadAPPerson(a app.Account) *pub.Actor {
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

func (r *repository) WithAccount(a *app.Account) error {
	r.client.SignFn(func(req *http.Request) error {
		// TODO(marius): this needs to be added to the federated requests, which we currently don't support
		if !a.IsValid() || !a.IsLogged() {
			e := errors.Newf("invalid account used for C2S authentication")
			r.logger.WithContext(log.Ctx{
				"handle": a.Handle,
				"logged": a.IsLogged(),
			}).Error(e.Error())
			return e
		}
		if a.Metadata.OAuth.Token == "" {
			e := errors.Newf("account has no OAuth2 token")
			r.logger.WithContext(log.Ctx{
				"handle": a.Handle,
				"logged": a.IsLogged(),
			}).Error(e.Error())
			return e
		}
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", a.Metadata.OAuth.Token))
		return nil
	})
	return nil
}

func (r *repository) withAccountS2S(a *app.Account) error {
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
	r.client.SignFn(s.Sign)

	return nil
}

func (r *repository) LoadItem(f app.Filters) (app.Item, error) {
	var item app.Item

	f.MaxItems = 1
	hashes := f.LoadItemsFilter.Key
	f.LoadItemsFilter.Key = nil

	url := fmt.Sprintf("%s/objects/%s", r.BaseURL, hashes[0])
	it, err := r.client.LoadIRI(pub.IRI(url))
	if err != nil {
		r.logger.Error(err.Error())
		return item, err
	}
	local.OnObject(it, func(art *local.Object) error {
		err = item.FromActivityPub(art)
		if err == nil {
			var items app.ItemCollection
			items, err = r.loadItemsAuthors(item)
			items, err = r.loadItemsVotes(items...)
			if len(items) > 0 {
				item = items[0]
			}
		}
		return nil
	})

	return item, err
}

func hashesUnique(a app.Hashes) app.Hashes {
	u := make([]app.Hash, 0, len(a))
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

func (r *repository) loadAccountsVotes(accounts ...app.Account) (app.AccountCollection, error) {
	if len(accounts) == 0 {
		return accounts, nil
	}
	fVotes := app.Filters{}
	for _, account := range accounts {
		fVotes.LoadVotesFilter.AttributedTo = append(fVotes.LoadVotesFilter.AttributedTo, account.Hash)
	}
	fVotes.LoadVotesFilter.AttributedTo = hashesUnique(fVotes.LoadVotesFilter.AttributedTo)
	col := make(app.AccountCollection, len(accounts))
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

func (r *repository) loadItemsVotes(items ...app.Item) (app.ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}
	fVotes := app.Filters{}
	for _, it := range items {
		fVotes.LoadVotesFilter.ItemKey = append(fVotes.LoadVotesFilter.ItemKey, it.Hash)
	}
	fVotes.LoadVotesFilter.ItemKey = hashesUnique(fVotes.LoadVotesFilter.ItemKey)
	col := make(app.ItemCollection, len(items))
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

func (r *repository) loadItemsAuthors(items ...app.Item) (app.ItemCollection, error) {
	if len(items) == 0 {
		return items, nil
	}

	fActors := app.Filters{}
	for _, it := range items {
		if it.HasMetadata() {
			// Adding an item's recipients list (To and CC) to the list of actors we want to load from the ActivityPub API
			if len(it.Metadata.To) > 0 {
				for _, to := range it.Metadata.To {
					fActors.LoadAccountsFilter.Key = append(fActors.LoadAccountsFilter.Key, app.Hash(to.Metadata.ID))
				}
			}
			if len(it.Metadata.CC) > 0 {
				for _, cc := range it.Metadata.CC {
					fActors.LoadAccountsFilter.Key = append(fActors.LoadAccountsFilter.Key, app.Hash(cc.Metadata.ID))
				}
			}
		}
		if !it.SubmittedBy.IsValid() {
			continue
		}
		// Adding an item's author to the list of actors we want to load from the ActivityPub API
		if it.SubmittedBy.HasMetadata() && len(it.SubmittedBy.Metadata.ID) > 0 {
			fActors.LoadAccountsFilter.Key = append(fActors.LoadAccountsFilter.Key, app.Hash(it.SubmittedBy.Metadata.ID))
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
	col := make(app.ItemCollection, len(items))
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

func accountsEqual(a1, a2 app.Account) bool {
	return bytes.Equal(a1.Hash, a2.Hash) || (len(a1.Handle) + len(a2.Handle) > 0 && a1.Handle == a2.Handle)
}

	func (r *repository) LoadItems(f app.Filters) (app.ItemCollection, uint, error) {
	var qs string

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
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}
	url := fmt.Sprintf("%s%s%s%s", r.BaseURL, target, c, qs)

	ctx := log.Ctx{
		"url": url,
	}

	it, err := r.client.LoadIRI(pub.IRI(url))
	if err != nil {
		r.logger.WithContext(ctx).Error(err.Error())
		return nil, 0, err
	}

	items := make(app.ItemCollection, 0)
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
			i := app.Item{}
			if err := i.FromActivityPub(it); err != nil {
				r.logger.Error(err.Error())
				continue
			}
			items = append(items, i)
		}
		return nil
	})

	// TODO(marius): move this somewhere more palatable
	//  it's currently done like this when loading from collections of Activities that only contain the ID of the object
	toLoad := make(app.Hashes, 0)
	for _, i := range items {
		if i.IsValid() && !i.Deleted() && i.SubmittedAt.IsZero() {
			toLoad = append(toLoad, app.Hash(i.Metadata.ID))
		}
	}
	if len(toLoad) > 0 {
		return r.LoadItems(app.Filters{
			LoadItemsFilter: app.LoadItemsFilter{
				Key: toLoad,
			},
		})
	}
	items, err = r.loadItemsAuthors(items...)
	items, err = r.loadItemsVotes(items...)

	return items, count, err
}

func (r *repository) SaveVote(v app.Vote) (app.Vote, error) {
	if !v.SubmittedBy.IsValid() || !v.SubmittedBy.HasMetadata() {
		return app.Vote{}, errors.Newf("Invalid vote submitter")
	}
	if !v.Item.IsValid() || !v.Item.HasMetadata() {
		return app.Vote{}, errors.Newf("Invalid vote item")
	}
	author := loadAPPerson(*v.SubmittedBy)
	reqURL := r.getAuthorRequestURL(v.SubmittedBy)

	url := fmt.Sprintf("%s/%s", v.Item.Metadata.ID, "likes")
	itemVotes, err := r.loadVotesCollection(pub.IRI(url), pub.IRI(v.SubmittedBy.Metadata.ID))
	// first step is to verify if vote already exists:
	if err != nil {
		r.logger.WithContext(log.Ctx{
			"url": url,
			"err": err,
		}).Warn(err.Error())
	}
	var exists app.Vote
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
		// undo old vote
		var body []byte
		if body, err = j.Marshal(act); err != nil {
			r.logger.Error(err.Error())
		}
		var resp *http.Response
		if resp, err = r.client.Post(reqURL, cl.ContentTypeActivityJson, bytes.NewReader(body)); err != nil {
			r.logger.Error(err.Error())
		}
		if body, err = ioutil.ReadAll(resp.Body); err != nil {
			r.logger.Error(err.Error())
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			r.logger.Error("unable to undo previous vote")
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

	var body []byte
	if body, err = j.Marshal(act); err != nil {
		r.logger.Error(err.Error())
		return v, err
	}

	var resp *http.Response
	if resp, err = r.client.Post(reqURL, cl.ContentTypeActivityJson, bytes.NewReader(body)); err != nil {
		r.logger.Error(err.Error())
		return v, err
	}
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return v, err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		err := v.FromActivityPub(act)
		return v, err
	}
	return v, r.handlerErrorResponse(body)
}

func (r *repository) loadVotesCollection(iri pub.IRI, actors ...pub.IRI) ([]app.Vote, error) {
	cntActors := len(actors)
	if cntActors > 0 {
		attrTo := make([]app.Hash, cntActors)
		for i, a := range actors {
			attrTo[i] = app.Hash(a.String())
		}
		f := app.LoadVotesFilter{
			AttributedTo: attrTo,
		}
		if q, err := qstring.MarshalString(&f); err == nil {
			iri = pub.IRI(fmt.Sprintf("%s?%s", iri, q))
		}
	}
	likes, err := r.client.LoadIRI(iri)
	// first step is to verify if vote already exists:
	if err != nil {
		return nil, err
	}
	votes := make([]app.Vote, 0)
	err = pub.OnOrderedCollection(likes, func(col *pub.OrderedCollection) error {
		for _, like := range col.OrderedItems {
			vote := app.Vote{}
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

func (r *repository) LoadVotes(f app.Filters) (app.VoteCollection, uint, error) {
	var qs string
	f.Type = pub.ActivityVocabularyTypes{
		pub.LikeType,
		pub.DislikeType,
		pub.UndoType,
	}

	var url string
	if len(f.LoadVotesFilter.AttributedTo) == 1 {
		attrTo := f.LoadVotesFilter.AttributedTo[0]
		f.LoadVotesFilter.AttributedTo = nil
		if q, err := qstring.MarshalString(&f); err == nil {
			qs = fmt.Sprintf("?%s", q)
		}
		url = fmt.Sprintf("%s/actors/%s/outbox%s", r.BaseURL, attrTo, qs)
	} else {
		if q, err := qstring.MarshalString(&f); err == nil {
			qs = fmt.Sprintf("?%s", q)
		}
		url = fmt.Sprintf("%s/inbox%s", r.BaseURL, qs)
	}

	it, err := r.client.LoadIRI(pub.IRI(url))
	if err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	var count uint = 0
	votes := make(app.VoteCollection, 0)
	undos := make(app.VoteCollection, 0)
	pub.OnOrderedCollection(it, func(col *pub.OrderedCollection) error {
		count = col.TotalItems
		for _, it := range col.OrderedItems {
			vot := app.Vote{}
			if err := vot.FromActivityPub(it); err != nil {
				r.logger.WithContext(log.Ctx{
					"type": fmt.Sprintf("%T", it),
				}).Warn(err.Error())
				continue
			}
			if vot.Weight != 0 {
				votes = append(votes, vot)
			} else {
				if vot.Metadata == nil || len(vot.Metadata.OriginalIRI) == 0 {
					r.logger.Error("Zero vote without an original activity undone")
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

func (r *repository) LoadVote(f app.Filters) (app.Vote, error) {
	if len(f.ItemKey) == 0 {
		return app.Vote{}, errors.Newf("invalid item hash")
	}

	v := app.Vote{}
	itemHash := f.ItemKey[0]
	f.ItemKey = nil
	url := fmt.Sprintf("%s/liked/%s", r.BaseURL, itemHash)

	it, err := r.client.LoadIRI(pub.IRI(url))
	if err != nil {
		r.logger.Error(err.Error())
		return v, err
	}
	err = pub.OnActivity(it, func(like *pub.Activity) error {
		return v.FromActivityPub(like)
	})
	return v, err
}

type _errors struct {
	Ctxt   string        `jsonld:"@context"`
	Errors []errors.Http `jsonld:"errors"`
}

func (r *repository) handlerErrorResponse(body []byte) error {
	errs := _errors{}
	if err := j.Unmarshal(body, &errs); err != nil {
		r.logger.Errorf("Unable to unmarshal error response: %s", err.Error())
		return nil
	}
	if len(errs.Errors) == 0 {
		return nil
	}
	err := errs.Errors[0]
	return errors.WrapWithStatus(err.Code, nil, err.Message)
}

func (r *repository) handleItemSaveSuccessResponse(it app.Item, body []byte) (app.Item, error) {
	ap, err := pub.UnmarshalJSON(body)
	if err != nil {
		r.logger.Error(err.Error())
		return it, err
	}
	err = it.FromActivityPub(ap)
	if err != nil {
		r.logger.Error(err.Error())
		return it, err
	}
	items, err := r.loadItemsAuthors(it)
	return items[0], err
}

func (r *repository) getAuthorRequestURL(a *app.Account) string {
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

func (r *repository) SaveItem(it app.Item) (app.Item, error) {
	if !it.SubmittedBy.IsValid() || !it.SubmittedBy.HasMetadata() {
		return app.Item{}, errors.Newf("Invalid item submitter")
	}
	art := loadAPItem(it)
	author := loadAPPerson(*it.SubmittedBy)
	reqURL := r.getAuthorRequestURL(it.SubmittedBy)

	to := make(pub.ItemCollection, 0)
	cc := make(pub.ItemCollection, 0)
	bcc := make(pub.ItemCollection, 0)

	var body []byte
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
			ff := app.Filters{
				LoadAccountsFilter: app.LoadAccountsFilter{
					Handle: names,
				},
			}
			actors, _, err := r.LoadAccounts(ff)
			if err != nil {
				r.logger.WithContext(log.Ctx{"err": err}).Error("unable to load actors from mentions")
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

	if it.Deleted() {
		if len(id) == 0 {
			r.logger.WithContext(log.Ctx{
				"item": it.Hash,
			}).Error(err.Error())
			return it, errors.NotFoundf("item hash is empty, can not delete")
		}
		delete := pub.Delete{
			Type:   pub.DeleteType,
			To:     to,
			CC:     cc,
			BCC:    bcc,
			Actor:  author.GetLink(),
			Object: id,
		}
		body, err = j.Marshal(delete)
	} else {
		if len(id) == 0 {
			create := pub.Create{
				Type:   pub.CreateType,
				To:     to,
				CC:     cc,
				BCC:    bcc,
				Actor:  author.GetLink(),
				Object: art,
			}
			body, err = j.Marshal(create)
		} else {
			update := pub.Update{
				Type:   pub.UpdateType,
				To:     to,
				CC:     cc,
				BCC:    bcc,
				Object: art,
				Actor:  author.GetLink(),
			}
			body, err = j.Marshal(update)
		}
	}

	if err != nil {
		r.logger.WithContext(log.Ctx{
			"item": it.Hash,
		}).Error(err.Error())
		return it, err
	}
	var resp *http.Response
	resp, err = r.client.Post(reqURL, cl.ContentTypeActivityJson, bytes.NewReader(body))
	if err != nil {
		r.logger.Error(err.Error())
		return it, err
	}
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return it, err
	}
	if resp.StatusCode >= 400 {
		return it, r.handlerErrorResponse(body)
	}
	return r.handleItemSaveSuccessResponse(it, body)
}

func (r *repository) LoadAccounts(f app.Filters) (app.AccountCollection, uint, error) {
	var qs string
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}
	url := fmt.Sprintf("%s/%s", ActorsURL, qs)

	it, err := r.client.LoadIRI(pub.IRI(url))
	if err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	accounts := make(app.AccountCollection, 0)
	var count uint = 0
	pub.OnOrderedCollection(it, func(col *pub.OrderedCollection) error {
		count = col.TotalItems
		for _, it := range col.OrderedItems {
			acc := app.Account{Metadata: &app.AccountMetadata{}}
			if err := acc.FromActivityPub(it); err != nil {
				r.logger.WithContext(log.Ctx{
					"type": fmt.Sprintf("%T", it),
				}).Warn(err.Error())
				continue
			}
			accounts = append(accounts, acc)
		}
		accounts, err = r.loadAccountsVotes(accounts...)
		return err
	})
	return accounts, count, nil
}

func (r *repository) LoadAccount(f app.Filters) (app.Account, error) {
	var accounts app.AccountCollection
	var err error
	if accounts, _, err = r.LoadAccounts(f); err != nil {
		return app.AnonymousAccount, err
	}

	var ac *app.Account
	if ac, err = accounts.First(); err != nil {
		var id string
		if len(f.LoadAccountsFilter.Key) > 0 {
			id = f.LoadAccountsFilter.Key[0].String()
		}
		if len(f.Handle) > 0 {
			id = f.Handle[0]
		}
		return app.AnonymousAccount, errors.NotFoundf("account %s", id)
	}
	return *ac, nil
}

func (r *repository) SaveAccount(a app.Account) (app.Account, error) {
	p := loadAPPerson(a)
	id := p.GetLink()

	p.Generator = pub.IRI(app.Instance.BaseURL)
	p.Published = time.Now()
	p.Updated = p.Published
	p.URL = accountURL(a)

	author := loadAPPerson(*a.CreatedBy)
	reqURL := author.Inbox.GetLink().String()

	var body []byte
	var err error
	now := time.Now()
	if a.Deleted() {
		if len(id) == 0 {
			r.logger.WithContext(log.Ctx{
				"account": a.Hash,
			}).Error(err.Error())
			return a, errors.NotFoundf("item hash is empty, can not delete")
		}
		delete := pub.Delete{
			Type:         pub.DeleteType,
			To:           pub.ItemCollection{pub.PublicNS},
			BCC:          pub.ItemCollection{pub.IRI(BaseURL)},
			AttributedTo: author.GetLink(),
			Updated:      now,
			Actor:        author.GetLink(),
			Object:       id,
		}
		body, err = j.Marshal(delete)
	} else {
		if len(id) == 0 {
			p.To = pub.ItemCollection{pub.PublicNS}
			p.BCC = pub.ItemCollection{pub.IRI(BaseURL)}
			create := pub.Create{
				Type:         pub.CreateType,
				To:           pub.ItemCollection{pub.PublicNS},
				BCC:          pub.ItemCollection{pub.IRI(BaseURL)},
				AttributedTo: author.GetLink(),
				Published:    now,
				Updated:      now,
				Actor:        author.GetLink(),
				Object:       p,
			}
			body, err = j.Marshal(create)
		} else {
			update := pub.Update{
				Type:         pub.UpdateType,
				To:           pub.ItemCollection{pub.PublicNS},
				BCC:          pub.ItemCollection{pub.IRI(BaseURL)},
				AttributedTo: author.GetLink(),
				Updated:      now,
				Object:       p,
				Actor:        author.GetLink(),
			}
			body, err = j.Marshal(update)
		}
	}
	if err != nil {
		r.logger.WithContext(log.Ctx{
			"account": a.Hash,
		}).Error(err.Error())
		return a, err
	}

	var resp *http.Response
	resp, err = r.client.Post(reqURL, cl.ContentTypeActivityJson, bytes.NewReader(body))
	if err != nil {
		r.logger.Error(err.Error())
		return a, err
	}
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return a, err
	}
	if resp.StatusCode >= 400 {
		return a, r.handlerErrorResponse(body)
	}
	ap, err := pub.UnmarshalJSON(body)
	if err != nil {
		r.logger.Error(err.Error())
		return a, err
	}
	err = a.FromActivityPub(ap)
	if err != nil {
		r.logger.Error(err.Error())
	}
	return a, err
}

// LoadInfo this method is here to keep compatibility with the repository interfaces
// but in the long term we might want to store some of this information in the DB
func (r *repository) LoadInfo() (app.Info, error) {
	return app.Instance.NodeInfo(), nil
}
