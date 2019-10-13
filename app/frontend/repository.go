package frontend

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	ap "github.com/go-ap/activitypub"
	cl "github.com/go-ap/activitypub/client"
	as "github.com/go-ap/activitystreams"
	"github.com/go-ap/auth"
	"github.com/go-ap/errors"
	j "github.com/go-ap/jsonld"
	"github.com/mariusor/littr.go/app"
	local "github.com/mariusor/littr.go/app/activitypub"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/mariusor/qstring"
	"github.com/spacemonkeygo/httpsig"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type repository struct {
	BaseURL string
	Account *app.Account
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
	as.ItemTyperFunc = local.JSONGetItemByType
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

func getObjectType(el as.Item) string {
	if el == nil {
		return ""
	}
	var label = ""
	switch el.(type) {
	case *ap.Outbox:
		label = "outbox"
	case ap.Outbox:
		label = "outbox"
	case *ap.Inbox:
		label = "inbox"
	case ap.Inbox:
		label = "inbox"
	case ap.Liked:
		label = "liked"
	case *ap.Liked:
		label = "liked"
	case ap.Followers:
		label = "followers"
	case *ap.Followers:
		label = "followers"
	case ap.Following:
		label = "following"
	case *ap.Following:
		label = "following"
	case as.Person:
		if o, ok := el.(as.Person); ok {
			label = o.Name.First().Value
		}
	case *as.Person:
		if o, ok := el.(*as.Person); ok {
			label = o.Name.First().Value
		}
	case ap.Person:
		if o, ok := el.(ap.Person); ok {
			label = o.Name.First().Value
		}
	case *ap.Person:
		if o, ok := el.(*ap.Person); ok {
			label = o.Name.First().Value
		}
	}
	return label
}

func BuildCollectionID(a app.Account, o as.Item) as.ObjectID {
	if len(a.Handle) > 0 {
		return as.ObjectID(fmt.Sprintf("%s/%s/%s", ActorsURL, url.PathEscape(a.Hash.String()), getObjectType(o)))
	}
	return as.ObjectID(fmt.Sprintf("%s/%s", BaseURL, getObjectType(o)))
}

var BaseURL = "http://fedbox.git"
var ActorsURL = "http://fedbox.git/actors"
var ObjectsURL = "http://fedbox.git/objects"

func apAccountID(a app.Account) as.ObjectID {
	if len(a.Hash) >= 8 {
		return as.ObjectID(fmt.Sprintf("%s/%s", ActorsURL, a.Hash.String()))
	}
	return as.ObjectID(fmt.Sprintf("%s/anonymous", ActorsURL))
}

func accountURL(acc app.Account) as.IRI {
	return as.IRI(fmt.Sprintf("%s%s", app.Instance.BaseURL, AccountPermaLink(acc)))
}

func BuildObjectIDFromItem(i app.Item) (as.ObjectID, bool) {
	if i.Hash == "" {
		return "", false
	}
	if i.HasMetadata() && len(i.Metadata.ID) > 0 {
		return as.ObjectID(i.Metadata.ID), true
	}
	return as.ObjectID(fmt.Sprintf("%s/%s", ObjectsURL, url.PathEscape(i.Hash.String()))), true
}

func BuildActorID(a app.Account) as.ObjectID {
	return as.ObjectID(fmt.Sprintf("%s/%s", ActorsURL, url.PathEscape(a.Hash.String())))
}

func loadAPItem(item app.Item) as.Item {
	o := local.Object{}

	if id, ok := BuildObjectIDFromItem(item); ok {
		o.ID = id
	}
	if item.MimeType == app.MimeTypeURL {
		o.Type = as.PageType
		o.URL = as.IRI(item.Data)
	} else {
		wordCount := strings.Count(item.Data, " ") +
			strings.Count(item.Data, "\t") +
			strings.Count(item.Data, "\n") +
			strings.Count(item.Data, "\r\n")
		if wordCount > 300 {
			o.Type = as.ArticleType
		} else {
			o.Type = as.NoteType
		}

		if len(item.Hash) > 0 {
			o.URL = as.IRI(ItemPermaLink(item))
		}
		o.Name = make(as.NaturalLanguageValues, 0)
		switch item.MimeType {
		case app.MimeTypeMarkdown:
			o.Object.Source.MediaType = as.MimeType(item.MimeType)
			o.MediaType = as.MimeType(app.MimeTypeHTML)
			if item.Data != "" {
				o.Source.Content.Set("en", string(item.Data))
				o.Content.Set("en", string(app.Markdown(string(item.Data))))
			}
		case app.MimeTypeText:
			fallthrough
		case app.MimeTypeHTML:
			o.MediaType = as.MimeType(item.MimeType)
			o.Content.Set("en", string(item.Data))
		}
	}

	// TODO(marius): add proper dynamic recipients to this based on some selector in the frontend
	o.To = as.ItemCollection{
		as.PublicNS,
		as.IRI(BaseURL),
	}
	o.Published = item.SubmittedAt
	o.Updated = item.UpdatedAt

	if item.Deleted() {
		del := as.Tombstone{
			Parent: as.Object{
				ID:   o.ID,
				Type: as.TombstoneType,
			},
			FormerType: o.Type,
			Deleted:    o.Updated,
		}
		if item.Parent != nil || item.OP != nil {
			repl := make(as.ItemCollection, 0)
			if item.Parent != nil {
				if par, ok := BuildObjectIDFromItem(*item.Parent); ok {
					repl = append(repl, as.IRI(par))
				}
			}
			if item.OP != nil {
				if op, ok := BuildObjectIDFromItem(*item.OP); ok {
					del.Context = as.IRI(op)
					repl = append(repl, as.IRI(op))
				}
			}
			if len(repl) > 0 {
				del.InReplyTo = repl
			}
		}

		return del
	}

	//o.Generator = as.IRI(app.Instance.BaseURL)
	o.Score = item.Score / app.ScoreMultiplier
	if item.Title != "" {
		o.Name.Set("en", string(item.Title))
	}
	if item.SubmittedBy != nil {
		id := BuildActorID(*item.SubmittedBy)
		o.AttributedTo = as.IRI(id)
	}
	if item.Parent != nil || item.OP != nil {
		repl := make(as.ItemCollection, 0)
		if item.Parent != nil {
			if par, ok := BuildObjectIDFromItem(*item.Parent); ok {
				repl = append(repl, as.IRI(par))
			}
		}
		if item.OP != nil {
			if op, ok := BuildObjectIDFromItem(*item.OP); ok {
				o.Context = as.IRI(op)
				repl = append(repl, as.IRI(op))
			}
		}
		if len(repl) > 0 {
			o.InReplyTo = repl
		}
	}

	if item.Metadata != nil {
		m := item.Metadata
		if m.Mentions != nil || m.Tags != nil {
			o.Tag = make(as.ItemCollection, 0)
			for _, men := range m.Mentions {
				t := as.Object{
					ID:   as.ObjectID(men.URL),
					Type: as.MentionType,
					Name: as.NaturalLanguageValues{{Ref: as.NilLangRef, Value: men.Name}},
				}
				o.Tag.Append(t)
			}
			for _, tag := range m.Tags {
				t := as.Object{
					ID:   as.ObjectID(tag.URL),
					Name: as.NaturalLanguageValues{{Ref: as.NilLangRef, Value: tag.Name}},
				}
				o.Tag.Append(t)
			}
		}
	}

	return &o
}

func loadAPPerson(a app.Account) *auth.Person {
	p := auth.Person{}
	p.Type = as.PersonType
	p.Name = as.NaturalLanguageValuesNew()
	p.PreferredUsername = as.NaturalLanguageValuesNew()

	if a.HasMetadata() {
		if a.Metadata.Blurb != nil && len(a.Metadata.Blurb) > 0 {
			p.Summary = as.NaturalLanguageValuesNew()
			p.Summary.Set(as.NilLangRef, string(a.Metadata.Blurb))
		}
		if len(a.Metadata.Icon.URI) > 0 {
			avatar := as.ObjectNew(as.ImageType)
			avatar.MediaType = as.MimeType(a.Metadata.Icon.MimeType)
			avatar.URL = as.IRI(a.Metadata.Icon.URI)
			p.Icon = avatar
		}
	}

	p.PreferredUsername.Set("en", a.Handle)

	if a.IsFederated() {
		p.ID = as.ObjectID(a.Metadata.ID)
		p.Name.Set("en", a.Metadata.Name)
		if len(a.Metadata.InboxIRI) > 0 {
			p.Inbox = as.IRI(a.Metadata.InboxIRI)
		}
		if len(a.Metadata.OutboxIRI) > 0 {
			p.Outbox = as.IRI(a.Metadata.OutboxIRI)
		}
		if len(a.Metadata.LikedIRI) > 0 {
			p.Liked = as.IRI(a.Metadata.LikedIRI)
		}
		if len(a.Metadata.FollowersIRI) > 0 {
			p.Followers = as.IRI(a.Metadata.FollowersIRI)
		}
		if len(a.Metadata.FollowingIRI) > 0 {
			p.Following = as.IRI(a.Metadata.FollowingIRI)
		}
		if len(a.Metadata.URL) > 0 {
			p.URL = as.IRI(a.Metadata.URL)
		}
	} else {
		p.Name.Set("en", a.Handle)

		p.Outbox = as.IRI(BuildCollectionID(a, new(ap.Outbox)))
		p.Inbox = as.IRI(BuildCollectionID(a, new(ap.Inbox)))
		p.Liked = as.IRI(BuildCollectionID(a, new(ap.Liked)))

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

	//p.Score = a.Score
	if a.IsValid() && a.HasMetadata() && a.Metadata.Key != nil && a.Metadata.Key.Public != nil {
		p.PublicKey = auth.PublicKey{
			ID:           as.ObjectID(fmt.Sprintf("%s#main-key", p.ID)),
			Owner:        as.IRI(p.ID),
			PublicKeyPem: fmt.Sprintf("-----BEGIN PUBLIC KEY-----\n%s\n-----END PUBLIC KEY-----", base64.StdEncoding.EncodeToString(a.Metadata.Key.Public)),
		}
	}
	oauthURL := strings.Replace(BaseURL, "api", "oauth", 1)
	p.Endpoints = &ap.Endpoints{
		SharedInbox:                as.IRI(fmt.Sprintf("%s/self/inbox", BaseURL)),
		OauthAuthorizationEndpoint: as.IRI(fmt.Sprintf("%s/authorize", oauthURL)),
		OauthTokenEndpoint:         as.IRI(fmt.Sprintf("%s/token", oauthURL)),
	}

	return &p
}

type SignFunc func(r *http.Request) error

func getSigner(pubKeyID as.ObjectID, key crypto.PrivateKey) *httpsig.Signer {
	hdrs := []string{"(request-target)", "host", "date"}
	return httpsig.NewSigner(string(pubKeyID), key, httpsig.RSASHA256, hdrs)
}

func (r *repository) WithAccount(a *app.Account) error {
	// TODO(marius): this needs to be added to the federated requests, which we currently don't support
	if !a.IsValid() || !a.IsLogged() {
		return nil
	}
	r.Account = a

	r.client.SignFn(func(r *http.Request) error {
		if a.Metadata.OAuth.Token == "" {
			return nil
		}
		r.Header.Add("Authorization", fmt.Sprintf("Bearer %s", a.Metadata.OAuth.Token))
		return nil
	})
	return nil
}

func (r *repository) withAccountS2S(a *app.Account) error {
	// TODO(marius): this needs to be added to the federated requests, which we currently don't support
	r.Account = a

	if r.Account == nil || r.Account.Hash == app.AnonymousAccount.Hash {
		return nil
	}

	k := r.Account.Metadata.Key
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
	p := *loadAPPerson(*r.Account)
	s := getSigner(p.PublicKey.ID, prv)
	r.client.SignFn(s.Sign)

	return nil
}

func (r *repository) LoadItem(f app.Filters) (app.Item, error) {
	var item app.Item

	f.MaxItems = 1
	hashes := f.LoadItemsFilter.Key
	f.LoadItemsFilter.Key = nil

	//var qs string
	//if q, err := qstring.MarshalString(&f); err == nil {
	//	qs = fmt.Sprintf("?%s", q)
	//}
	url := fmt.Sprintf("%s/objects/%s", r.BaseURL, hashes[0])

	var err error
	var resp *http.Response
	if resp, err = r.client.Get(url); err != nil {
		r.logger.Error(err.Error())
		return item, err
	}
	if resp == nil {
		err := errors.Newf("empty response from the repository")
		r.logger.Error(err.Error())
		return item, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.Newf("unable to load item from the API")
		r.logger.Error(err.Error())
		return item, err
	}
	defer resp.Body.Close()
	var body []byte

	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return item, err
	}

	it, err := as.UnmarshalJSON(body)
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
	m := make(map[app.Hash]bool)

	for _, val := range a {
		if _, ok := m[val]; !ok {
			m[val] = true
			u = append(u, val)
		}
	}
	return u
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
		return nil, errors.Annotatef(err, "unable to load items votes")
	}
	for k, it := range items {
		for _, vot := range votes {
			if vot.Item.Hash == it.Hash {
				it.Score += vot.Weight
			}
		}
		col[k] = it
	}
	return col, nil
}

func (r *repository) loadItemsAuthors(items ...app.Item) (app.ItemCollection, error) {
	if len(items) == 0 {
		return app.ItemCollection{}, nil
	}

	fActors := app.Filters{}
	for _, it := range items {
		if len(it.SubmittedBy.Hash) > 0 {
			fActors.LoadAccountsFilter.Key = append(fActors.LoadAccountsFilter.Key, it.SubmittedBy.Hash)
		} else if len(it.SubmittedBy.Handle) > 0 {
			fActors.Handle = append(fActors.Handle, it.SubmittedBy.Handle)
		}
	}
	fActors.LoadAccountsFilter.Key = hashesUnique(fActors.LoadAccountsFilter.Key)
	if len(fActors.LoadAccountsFilter.Key)+len(fActors.Handle) == 0 {
		return nil, errors.Errorf("unable to load items authors")
	}
	col := make(app.ItemCollection, len(items))
	authors, _, err := r.LoadAccounts(fActors)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to load items authors")
	}
	for k, it := range items {
		for _, auth := range authors {
			if it.SubmittedBy.Hash == auth.Hash || it.SubmittedBy.Handle == auth.Handle {
				it.SubmittedBy = &auth
				break
			}
		}
		col[k] = it
	}
	return col, nil
}

func (r *repository) LoadItems(f app.Filters) (app.ItemCollection, uint, error) {
	var qs string

	target := "/"
	c := "objects"
	if len(f.FollowedBy) > 0 {
		// TODO(marius): make this work for multiple FollowedBy filters
		for _, foll := range f.FollowedBy {
			target = fmt.Sprintf("/following/%s", foll)
			c = "inbox"
			break
		}
		f.FollowedBy = f.FollowedBy[:0]
	}
	if len(f.Context) > 0 {
		// TODO(marius): make this work for multiple Context filters
		for _, ctxt := range f.Context {
			if ctxt != "0" {
				c = fmt.Sprintf("%s/%s/replies", c, ctxt)
				f.Context = f.Context[:0]
			}
			break
		}
	}
	//if len(f.InReplyTo) > 0 {
	//	// TODO(marius): make this work for multiple Context filters
	//	for _, ctxt := range f.InReplyTo {
	//		if ctxt != "0" {
	//			c = fmt.Sprintf("%s/%s/replies", c, ctxt)
	//			f.Context = f.Context[:0]
	//		}
	//		break
	//	}
	//}
	if len(f.Federated) > 0 {
		for _, fed := range f.Federated {
			if !fed {
				// TODO(marius): need to add to fedbox support for filtering by hostname
				f.LoadItemsFilter.IRI = BaseURL
				break
			}
		}
	}
	if len(f.LoadItemsFilter.Deleted) > 0 {
		f.Type = as.ActivityVocabularyTypes{
			as.ArticleType,
			as.AudioType,
			as.DocumentType,
			as.ImageType,
			as.NoteType,
			as.PageType,
			as.VideoType,
		}
	}
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}
	url := fmt.Sprintf("%s%s%s%s", r.BaseURL, target, c, qs)

	var err error
	var resp *http.Response
	ctx := log.Ctx{
		"url": url,
	}
	if resp, err = r.client.Get(url); err != nil {
		r.logger.WithContext(ctx).Error(err.Error())
		return nil, 0, err
	}

	ctx["authCode"] = resp.StatusCode
	ctx["status"] = resp.Status
	if resp == nil {
		err := fmt.Errorf("nil response from the repository")
		r.logger.WithContext(ctx).Error(err.Error())
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.Newf("unable to load items from the API")
		r.logger.WithContext(ctx).Error(err.Error())
		return nil, 0, err
	}
	defer resp.Body.Close()
	var body []byte

	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	it, err := as.UnmarshalJSON(body)
	if err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}

	items := make(app.ItemCollection, 0)
	var count uint = 0
	ap.OnOrderedCollection(it, func(col *as.OrderedCollection) error {
		count = col.TotalItems
		for _, it := range col.OrderedItems {
			i := app.Item{}
			if err := i.FromActivityPub(it); err != nil {
				r.logger.Error(err.Error())
				continue
			}
			items = append(items, i)
		}
		items, err = r.loadItemsAuthors(items...)
		items, err = r.loadItemsVotes(items...)
		return err
	})

	return items, count, err
}

func (r *repository) loadVotes(iri as.IRI) ([]app.Vote, error) {
	likes, err := r.client.LoadIRI(iri)
	// first step is to verify if vote already exists:
	if err != nil {
		return nil, err
	}
	allVotes := make([]app.Vote, 0)
	err = ap.OnOrderedCollection(likes, func(col *as.OrderedCollection) error {
		for _, like := range col.OrderedItems {
			vote := app.Vote{}
			vote.FromActivityPub(like)
			allVotes = append(allVotes, vote)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	votes := make([]app.Vote, 0)
	for _, vot := range allVotes {
		skip := false
		for i, cursor := range votes {
			if vot.SubmittedBy.Hash == cursor.SubmittedBy.Hash {
				votes[i].Weight += vot.Weight
				skip = true
				continue
			}
		}
		if !skip {
			votes = append(votes, vot)
		}
	}

	return votes, nil
}

func (r *repository) SaveVote(v app.Vote) (app.Vote, error) {
	if v.SubmittedBy == nil || v.SubmittedBy.Metadata == nil {
		return app.Vote{}, errors.Newf("Invalid vote submitter")
	}
	if v.Item == nil || v.Item.Metadata == nil {
		return app.Vote{}, errors.Newf("Invalid vote item")
	}
	var p *auth.Person
	if r.Account != nil && r.Account.Hash == v.SubmittedBy.Hash {
		p = loadAPPerson(*v.SubmittedBy)
	} else {
		p = loadAPPerson(app.AnonymousAccount)
	}

	url := fmt.Sprintf("%s/%s", v.Item.Metadata.ID, "likes")

	itemVotes, err := r.loadVotes(as.IRI(url))
	// first step is to verify if vote already exists:
	if err != nil {
		r.logger.WithContext(log.Ctx{
			"url": url,
			"err": err,
		}).Warn(err.Error())
	}
	var exists app.Vote
	for _, vot := range itemVotes {
		if vot.SubmittedBy.Hash == v.SubmittedBy.Hash {
			exists = vot
			break
		}
	}
	o := loadAPItem(*v.Item)
	//id := as.ObjectID(url)

	act := as.Activity{
		Parent: as.Object{
			Type: as.UndoType,
			To:   as.ItemCollection{as.PublicNS, as.IRI(BaseURL),},
		},
		Actor:  p.GetLink(),
		Object: o.GetLink(),
	}

	if v.Weight > 0 && exists.Weight <= 0 {
		act.Type = as.LikeType
	}
	if v.Weight < 0 && exists.Weight >= 0 {
		act.Type = as.DislikeType
	}

	var body []byte
	if body, err = j.Marshal(act); err != nil {
		r.logger.Error(err.Error())
		return v, err
	}

	var resp *http.Response
	if resp, err = r.client.Post(p.Outbox.GetLink().String(), cl.ContentTypeActivityJson, bytes.NewReader(body)); err != nil {
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

func (r *repository) LoadVotes(f app.Filters) (app.VoteCollection, uint, error) {
	var qs string
	f.Type = as.ActivityVocabularyTypes{
		as.LikeType,
		as.DislikeType,
		as.UndoType,
	}
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	var err error
	var resp *http.Response
	url := fmt.Sprintf("%s/activities%s", r.BaseURL, qs)
	if resp, err = r.client.Get(url); err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	if resp == nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.Newf("unable to load votes from the API")
		r.logger.Error(err.Error())
		return nil, 0, err
	}

	defer resp.Body.Close()
	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	it, err := as.UnmarshalJSON(body)
	if err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	votes := make(app.VoteCollection, 0)
	var count uint = 0
	ap.OnOrderedCollection(it, func(col *as.OrderedCollection) error {
		count = col.TotalItems
		for _, it := range col.OrderedItems {
			vot := app.Vote{}
			if err := vot.FromActivityPub(it); err != nil {
				r.logger.WithContext(log.Ctx{
					"type": fmt.Sprintf("%T", it),
				}).Warn(err.Error())
				continue
			}
			votes = append(votes, vot)
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

	var err error
	var resp *http.Response
	if resp, err = r.client.Get(url); err != nil {
		r.logger.Error(err.Error())
		return v, err
	}
	if resp == nil {
		err := errors.Newf("nil response from the repository")
		r.logger.Error(err.Error())
		return v, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.Newf("unable to load vote from the API")
		r.logger.Error(err.Error())
		return v, err
	}
	defer resp.Body.Close()

	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return v, err
	}

	it, err := as.UnmarshalJSON(body)
	if err != nil {
		r.logger.Error(err.Error())
		return v, err
	}
	err = ap.OnActivity(it, func(like *as.Activity) error {
		return v.FromActivityPub(like)
	})
	return v, err
}

type _errors struct {
	Ctxt string `jsonld:"@context"`
	Errors []errors.Http `jsonld:"errors"`
}

func (r *repository) handlerErrorResponse(body []byte) error {
	errs := _errors{}
	if err := j.Unmarshal(body, &errs); err != nil {
		r.logger.Errorf("Unable to unmarshall error response: %s", err.Error())
		return nil
	}
	if len(errs.Errors) == 0 {
		return nil
	}
	err := errs.Errors[0]
	return errors.WrapWithStatus(err.Code, nil, err.Message)
}

func (r *repository) handleSuccessResponse(it app.Item, body []byte) (app.Item, error) {
	ap, err := as.UnmarshalJSON(body)
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

func (r *repository) SaveItem(it app.Item) (app.Item, error) {
	art := loadAPItem(it)

	var actor *auth.Person
	if r.Account != nil && r.Account.Hash == it.SubmittedBy.Hash {
		actor = loadAPPerson(*it.SubmittedBy)
	} else {
		actor = loadAPPerson(app.AnonymousAccount)
	}

	var body []byte
	var err error
	id := art.GetLink()
	if it.Deleted() {
		if len(id) == 0 {
			r.logger.WithContext(log.Ctx{
				"item": it.Hash,
			}).Error(err.Error())
			return it, errors.NotFoundf("item hash is empty, can not delete")
		}
		delete := as.Delete{
			Parent: as.Parent{
				Type: as.DeleteType,
				To:   as.ItemCollection{as.PublicNS, as.IRI(BaseURL),},
			},
			Actor:  actor.GetLink(),
			Object: id,
		}
		body, err = j.Marshal(delete)
	} else {
		if len(id) == 0 {
			create := as.Create{
				Parent: as.Parent{
					Type: as.CreateType,
					To:   as.ItemCollection{as.PublicNS, as.IRI(BaseURL),},
				},
				Actor:  actor.GetLink(),
				Object: art,
			}
			body, err = j.Marshal(create)
		} else {
			update := as.Update{
				Parent: as.Parent{
					Type: as.UpdateType,
					To:   as.ItemCollection{as.PublicNS, as.IRI(BaseURL),},
				},
				Object: art,
				Actor:  actor.GetLink(),
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
	resp, err = r.client.Post(actor.Outbox.GetLink().String(), cl.ContentTypeActivityJson, bytes.NewReader(body))
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
	return r.handleSuccessResponse(it, body)
}

func (r *repository) Get(u string) (*http.Response, error) {
	return r.client.Get(u)
}

func (r *repository) LoadAccounts(f app.Filters) (app.AccountCollection, uint, error) {
	var qs string
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}
	url := fmt.Sprintf("%s/%s", ActorsURL, qs)

	var err error
	var resp *http.Response
	if resp, err = r.client.Get(url); err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	if resp == nil {
		err := fmt.Errorf("nil response from the repository")
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.Newf("unable to load accounts from the API")
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	defer resp.Body.Close()
	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	it, err := as.UnmarshalJSON(body)
	if err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	accounts := make(app.AccountCollection, 0)
	var count uint = 0
	ap.OnOrderedCollection(it, func(col *as.OrderedCollection) error {
		count = col.TotalItems
		for _, it := range col.OrderedItems {
			acc := app.Account{}
			if err := acc.FromActivityPub(it); err != nil {
				r.logger.WithContext(log.Ctx{
					"type": fmt.Sprintf("%T", it),
				}).Warn(err.Error())
				continue
			}
			accounts = append(accounts, acc)
		}
		return err
	})
	return accounts, count, nil
}

func (r *repository) LoadAccount(f app.Filters) (app.Account, error) {
	var accounts app.AccountCollection
	var err error
	if accounts, _, err = r.LoadAccounts(f); err != nil {
		return app.Account{}, err
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
		return app.Account{}, errors.NotFoundf("account %s", id)
	}
	return *ac, nil
}

func (r *repository) SaveAccount(a app.Account) (app.Account, error) {
	return db.Config.SaveAccount(a)
}

// LoadInfo this method is here to keep compatibility with the repository interfaces
// but in the long term we might want to store some of this information in the DB
func (r *repository) LoadInfo() (app.Info, error) {
	return app.Instance.NodeInfo(), nil
}
