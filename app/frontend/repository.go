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
	"path"
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
	return &repository{
		BaseURL: c.BaseURL,
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
	if len(i.Hash) == 0 {
		return as.ObjectID(""), false
	}
	if i.SubmittedBy != nil {
		hash := i.SubmittedBy.Hash
		return as.ObjectID(fmt.Sprintf("%s/%s/outbox/%s/object", ActorsURL, url.PathEscape(hash.String()), url.PathEscape(i.Hash.String()))), true
	} else {
		return as.ObjectID(fmt.Sprintf("%s/self/outbox/%s/object", BaseURL, url.PathEscape(i.Hash.String()))), true
	}
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
		if item.Parent != nil {
			if par, ok := BuildObjectIDFromItem(*item.Parent); ok {
				del.InReplyTo = as.IRI(par)
			}
		}
		if item.OP != nil {
			if op, ok := BuildObjectIDFromItem(*item.OP); ok {
				del.Context = as.IRI(op)
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
	if item.Parent != nil {
		id, _ := BuildObjectIDFromItem(*item.Parent)
		o.InReplyTo = as.IRI(id)
	}
	if item.OP != nil {
		id, _ := BuildObjectIDFromItem(*item.OP)
		o.Context = as.IRI(id)
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
		err := errors.Newf("unable to load from the API")
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
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	target := "/"
	c := "objects"
	if len(f.FollowedBy) > 0 {
		for _, foll := range f.FollowedBy {
			target = fmt.Sprintf("/following/%s", foll)
			c = "inbox"
			break
		}
	}

	if len(f.Federated) > 0 {
		// TODO(marius): need to add to fedbox support for filtering by hostname
	}
	url := fmt.Sprintf("%s%s/%s%s", r.BaseURL, target, c, qs)

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
		err := errors.Newf("unable to load from the API")
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
		return err
	})

	return items, count, err
}

func (r *repository) SaveVote(v app.Vote) (app.Vote, error) {
	url := fmt.Sprintf("%s/self/following/%s/liked/%s", r.BaseURL, v.SubmittedBy.Hash, v.Item.Hash)

	var err error
	var exists as.Item
	// first step is to verify if vote already exists:
	if exists, err = r.client.LoadIRI(as.IRI(url)); err != nil {
		r.logger.WithContext(log.Ctx{
			"url": url,
			"err": err,
		}).Warn(err.Error())
	}
	p := loadAPPerson(*v.SubmittedBy)
	o := loadAPItem(*v.Item)
	id := as.ObjectID(url)

	var act as.Activity
	act.ID = id
	act.Actor = p.GetLink()
	act.Object = o.GetLink()
	act.Type = as.UndoType

	if v.Weight > 0 && (exists == nil || len(*exists.GetID()) == 0 || exists.GetType() != as.LikeType) {
		act.Type = as.LikeType
	}
	if v.Weight < 0 && (exists == nil || len(*exists.GetID()) == 0 || exists.GetType() != as.DislikeType) {
		act.Type = as.DislikeType
	}

	var body []byte
	if body, err = j.Marshal(act); err != nil {
		r.logger.Error(err.Error())
		return v, err
	}

	var resp *http.Response
	outbox := fmt.Sprintf("%s/self/following/%s/outbox", r.BaseURL, v.Item.SubmittedBy.Hash)
	if resp, err = r.client.Post(outbox, "application/json+activity", bytes.NewReader(body)); err != nil {
		r.logger.Error(err.Error())
		return v, err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		err := v.FromActivityPub(act)
		return v, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return v, errors.Errorf("vote not found")
	}
	if resp.StatusCode == http.StatusInternalServerError {
		return v, errors.Errorf("unable to save vote")
	}
	return v, errors.Errorf("unknown error, received status %d", resp.StatusCode)
}

func (r *repository) LoadVotes(f app.Filters) (app.VoteCollection, uint, error) {
	var qs string
	//f.Type = app.ActivityTypes{
	//	app.TypeLike,
	//	app.TypeDislike,
	//	app.TypeUndo,
	//}
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	var err error
	var resp *http.Response
	url := fmt.Sprintf("%s/self/liked%s", r.BaseURL, qs)
	//url := fmt.Sprintf("%s/self/inbox%s", r.BaseURL, qs)
	if resp, err = r.client.Get(url); err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	if resp == nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.Newf("unable to load from the API")
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
		err := errors.Newf("unable to load from the API")
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
		return  v.FromActivityPub(like)
	})
	return v, err
}

func (r *repository) SaveItem(it app.Item) (app.Item, error) {
	art := loadAPItem(it)

	actor := loadAPPerson(app.AnonymousAccount)
	if r.Account != nil && r.Account.Hash == it.SubmittedBy.Hash {
		// need to test if it.SubmittedBy matches r.Account and that the signature is valid
		actor = loadAPPerson(*it.SubmittedBy)
	}

	var body []byte
	var err error
	if it.Deleted() {
		if len(*art.GetID()) == 0 {
			r.logger.WithContext(log.Ctx{
				"item": it.Hash,
			}).Error(err.Error())
			return it, errors.NotFoundf("item hash is empty, can not delete")
		}
		id := art.GetID()
		delete := as.DeleteNew(*id, art)
		delete.Actor = actor.GetLink()
		body, err = j.Marshal(delete)
	} else {
		if len(*art.GetID()) == 0 {
			id := as.ObjectID("")
			create := as.CreateNew(id, art)
			create.Actor = actor.GetLink()
			body, err = j.Marshal(create)
		} else {
			id := art.GetID()
			update := as.UpdateNew(*id, art)
			update.Actor = actor.GetLink()
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
	url := fmt.Sprintf("%s/self/following/%s/outbox", r.BaseURL, it.SubmittedBy.Hash)
	resp, err = r.client.Post(url, "application/activity+json", bytes.NewReader(body))
	if err != nil {
		r.logger.Error(err.Error())
		return it, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		err := it.FromActivityPub(art)
		return it, err
	case http.StatusCreated:
		newLoc := resp.Header.Get("Location")
		hash := path.Base(newLoc)
		f := app.Filters{LoadItemsFilter: app.LoadItemsFilter{
			Key: app.Hashes{app.Hash(hash)},
		}}
		return r.LoadItem(f)
	case http.StatusGone:
		newLoc := resp.Header.Get("Location")
		hash := path.Base(newLoc)
		f := app.Filters{LoadItemsFilter: app.LoadItemsFilter{
			Key: app.Hashes{app.Hash(hash)},
		}}
		return r.LoadItem(f)
	case http.StatusNotFound:
		return it, errors.Errorf("%s", resp.Status)
	case http.StatusMethodNotAllowed:
		return it, errors.Errorf("%s", resp.Status)
	case http.StatusInternalServerError:
		return it, errors.Errorf("unable to save item %s", resp.Status)
	default:
		return app.Item{}, errors.Errorf("unknown error, received status %d", resp.StatusCode)
	}
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
		err := errors.Newf("unable to load from the API")
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
