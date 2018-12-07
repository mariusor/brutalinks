package api

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strings"

	"github.com/spacemonkeygo/httpsig"

	"github.com/mariusor/littr.go/app"
	ap "github.com/mariusor/littr.go/app/activitypub"
	"github.com/mariusor/littr.go/app/frontend"

	"github.com/mariusor/qstring"

	"github.com/go-chi/chi"
	"github.com/juju/errors"
	as "github.com/mariusor/activitypub.go/activitystreams"
	j "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/log"
)

type repository struct {
	BaseURL string
	Account *app.Account
	logger  log.Logger
	client  ap.Client
}

func New(c Config) *repository {
	return &repository{
		BaseURL: c.BaseURL,
		logger:  c.Logger,
		client: ap.NewClient(ap.Config{
			Logger:    c.Logger,
			UserAgent: fmt.Sprintf("%s-%s", app.Instance.HostName, app.Instance.Version),
		}),
	}
}

func (r *repository) WithAccount(a *app.Account) error {
	r.Account = a

	if r.Account == nil || r.Account.Hash == frontend.AnonymousAccount().Hash {
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
	return r.client.WithSigner(getSigner(p.PublicKey.ID, prv))
}

// Repository middleware
func (h handler) Repository(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), app.RepositoryCtxtKey, h.repo)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func ServiceCtxt(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func (h handler) AccountCtxt(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		handle := chi.URLParam(r, "handle")
		val := r.Context().Value(app.RepositoryCtxtKey)
		AcctLoader, ok := val.(app.CanLoadAccounts)
		if !ok {
			h.logger.Error("could not load account repository from Context")
		}
		a, err := AcctLoader.LoadAccount(app.LoadAccountsFilter{Handle: []string{handle}})
		if err == nil {
			// we redirect to the Hash based account URL
			url := strings.Replace(r.RequestURI, a.Handle, a.Hash.String(), 1)
			http.Redirect(w, r, url, http.StatusSeeOther)
			return
		} else {
			a, err := AcctLoader.LoadAccount(app.LoadAccountsFilter{Key: []string{handle}})
			if err != nil {
				h.logger.Error(err.Error())
				h.HandleError(w, r, errors.NewNotFound(err, "not found"))
				return
			}
			if a.Handle == "" && len(a.Hash) == 0 {
				h.HandleError(w, r, errors.NotFoundf("account not found"))
				return
			}
			ctx := context.WithValue(r.Context(), app.AccountCtxtKey, a)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
	return http.HandlerFunc(fn)
}

func (h handler) ItemCtxt(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		col := chi.URLParam(r, "collection")

		f := r.Context().Value(app.FilterCtxtKey)
		val := r.Context().Value(app.RepositoryCtxtKey)

		var err error
		var i interface{}
		if col == "outbox" {
			filters, ok := f.(app.LoadItemsFilter)
			if !ok {
				h.logger.Error("could not load item filter from Context")
			}
			loader, ok := val.(app.CanLoadItems)
			if !ok {
				h.logger.Error("could not load item repository from Context")
				h.HandleError(w, r, errors.NewNotValid(err, "not found"))
				return
			}
			i, err = loader.LoadItem(filters)
			if err != nil {
				h.logger.Error(err.Error())
				h.HandleError(w, r, errors.NewNotFound(err, "not found"))
				return
			}
		}
		if col == "liked" {
			filters, ok := f.(app.LoadVotesFilter)
			if !ok {
				h.logger.Error("could not load vote filter from Context")
			}
			loader, ok := val.(app.CanLoadVotes)
			if !ok {
				h.logger.Error("could not load vote repository from Context")
				h.HandleError(w, r, errors.NewNotValid(err, "not found"))
				return
			}
			i, err = loader.LoadVote(filters)
			if err != nil {
				h.logger.Error(err.Error())
				h.HandleError(w, r, errors.NewNotFound(err, "not found"))
				return
			}
		}

		ctx := context.WithValue(r.Context(), app.ItemCtxtKey, i)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func loadPersonFiltersFromReq(r *http.Request) app.LoadAccountsFilter {
	filters := app.LoadAccountsFilter{
		MaxItems: MaxContentItems,
	}

	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return filters
	}
	return filters
}

func loadRepliesFilterFromReq(r *http.Request) app.LoadItemsFilter {
	filters := app.LoadItemsFilter{
		MaxItems: MaxContentItems,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return filters
	}
	hash := chi.URLParam(r, "hash")

	filters.InReplyTo = []string{hash}

	return filters
}

func loadInboxFilterFromReq(r *http.Request) app.LoadItemsFilter {
	filters := app.LoadItemsFilter{
		MaxItems: MaxContentItems,
	}

	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return filters
	}
	handle := chi.URLParam(r, "handle")
	if handle != "" {
		old := filters.FollowedBy
		filters.FollowedBy = nil
		filters.FollowedBy = append(filters.FollowedBy, handle)
		filters.FollowedBy = append(filters.FollowedBy, old...)
	}
	hash := chi.URLParam(r, "hash")
	if hash != "" {
		old := filters.Key
		filters.Key = nil
		filters.Key = append(filters.Key, hash)
		filters.Key = append(filters.Key, old...)
	}

	return filters
}

func loadOutboxFilterFromReq(r *http.Request) app.LoadItemsFilter {
	filters := app.LoadItemsFilter{
		MaxItems: MaxContentItems,
	}

	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return filters
	}
	handle := chi.URLParam(r, "handle")
	if handle != "" {
		old := filters.AttributedTo
		filters.AttributedTo = nil
		filters.AttributedTo = append(filters.AttributedTo, app.Hash(handle))
		filters.AttributedTo = append(filters.AttributedTo, old...)
	}
	hash := chi.URLParam(r, "hash")
	if hash != "" {
		old := filters.Key
		filters.Key = nil
		filters.Key = append(filters.Key, hash)
		filters.Key = append(filters.Key, old...)
	}

	return filters
}

func loadLikedFilterFromReq(r *http.Request) app.LoadVotesFilter {
	filters := app.LoadVotesFilter{}
	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return filters
	}

	handle := chi.URLParam(r, "handle")
	if handle != "" {
		old := filters.AttributedTo
		filters.AttributedTo = nil
		filters.AttributedTo = append(filters.AttributedTo, app.Hash(handle))
		filters.AttributedTo = append(filters.AttributedTo, old...)
	}
	hash := chi.URLParam(r, "hash")
	if hash != "" {
		old := filters.ItemKey
		filters.ItemKey = nil
		filters.ItemKey = append(filters.ItemKey, hash)
		filters.ItemKey = append(filters.ItemKey, old...)
	}
	if filters.MaxItems == 0 {
		if len(filters.ItemKey) > 0 {
			filters.MaxItems = 1
		} else {
			filters.MaxItems = MaxContentItems
		}
	}
	return filters
}

func LoadFiltersCtxt(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		col := getCollectionFromReq(r)
		var filters interface{}
		switch col {
		case "disliked":
			fallthrough
		case "liked":
			filters = loadLikedFilterFromReq(r)
		case "outbox":
			filters = loadOutboxFilterFromReq(r)
		case "inbox":
			filters = loadInboxFilterFromReq(r)
		case "replies":
			filters = loadRepliesFilterFromReq(r)
		case "":
			filters = loadPersonFiltersFromReq(r)
		}

		ctx := context.WithValue(r.Context(), app.FilterCtxtKey, filters)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func (h handler) ItemCollectionCtxt(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		var err error
		col := getCollectionFromReq(r)

		f := r.Context().Value(app.FilterCtxtKey)
		val := r.Context().Value(app.RepositoryCtxtKey)

		var items interface{}
		if col == "inbox" {
			filters, ok := f.(app.LoadItemsFilter)
			if !ok {
				h.logger.Error("could not load item filters from Context")
				next.ServeHTTP(w, r)
				return
			}
			loader, ok := val.(app.CanLoadItems)
			if !ok {
				h.logger.Error("could not load item repository from Context")
				next.ServeHTTP(w, r)
				return
			}
			items, err = loader.LoadItems(filters)
			if err != nil {
				h.logger.Error(err.Error())
				next.ServeHTTP(w, r)
				return
			}
		}
		if col == "outbox" {
			filters, ok := f.(app.LoadItemsFilter)
			if !ok {
				h.logger.Error("could not load item filters from Context")
				next.ServeHTTP(w, r)
				return
			}
			loader, ok := val.(app.CanLoadItems)
			if !ok {
				h.logger.Error("could not load item repository from Context")
				next.ServeHTTP(w, r)
				return
			}
			items, err = loader.LoadItems(filters)
			if err != nil {
				h.logger.Error(err.Error())
				next.ServeHTTP(w, r)
				return
			}
		}
		if col == "liked" {
			filters, ok := f.(app.LoadVotesFilter)
			if !ok {
				h.logger.Error("could not load votes filters from Context")
				next.ServeHTTP(w, r)
				return
			}
			loader, ok := val.(app.CanLoadVotes)
			if !ok {
				h.logger.Error("could not load vote repository from Context")
				next.ServeHTTP(w, r)
				return
			}
			items, err = loader.LoadVotes(filters)
			if err != nil {
				h.logger.Error(err.Error())
				next.ServeHTTP(w, r)
				return
			}
		}
		if col == "replies" {
			filters, ok := f.(app.LoadItemsFilter)
			if !ok {
				h.logger.Error("could not load item filters from Context")
				next.ServeHTTP(w, r)
				return
			}
			loader, ok := val.(app.CanLoadItems)
			if !ok {
				h.logger.Error("could not load item repository from Context")
				next.ServeHTTP(w, r)
				return
			}
			items, err = loader.LoadItems(filters)
			if err != nil {
				h.logger.Error(err.Error())
				next.ServeHTTP(w, r)
				return
			}
		}

		ctx := context.WithValue(r.Context(), app.CollectionCtxtKey, items)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func (r *repository) LoadItem(f app.LoadItemsFilter) (app.Item, error) {
	var art ap.Article
	var it app.Item

	f.MaxItems = 1
	f.AttributedTo = nil
	hashes := f.Key
	f.Key = nil

	var qs string
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}
	url := fmt.Sprintf("%s/self/outbox/%s/object%s", r.BaseURL, hashes[0], qs)

	var err error
	var resp *http.Response
	if resp, err = r.client.Get(url); err != nil {
		r.logger.Error(err.Error())
		return it, err
	}
	if resp == nil {
		err := errors.New("nil response from the repository")
		r.logger.Error(err.Error())
		return it, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.New("unable to load from the API")
		r.logger.Error(err.Error())
		return it, err
	}
	defer resp.Body.Close()
	var body []byte

	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return it, err
	}
	if err := j.Unmarshal(body, &art); err != nil {
		r.logger.Error(err.Error())
		return it, err
	}
	err = it.FromActivityPub(art)
	if err == nil {
		var items app.ItemCollection
		items, err = r.loadItemsAuthors(it)
		if len(items) > 0 {
			it = items[0]
		}
	}
	return it, err
}

func (r *repository) loadItemsAuthors(items ...app.Item) (app.ItemCollection, error) {
	if len(items) == 0 {
		return app.ItemCollection{}, nil
	}

	fActors := app.LoadAccountsFilter{}
	for _, it := range items {
		if len(it.SubmittedBy.Hash) > 0 {
			fActors.Key = append(fActors.Key, it.SubmittedBy.Hash.String())
		} else if len(it.SubmittedBy.Handle) > 0 {
			fActors.Handle = append(fActors.Handle, it.SubmittedBy.Handle)
		}
	}
	if len(fActors.Key)+len(fActors.Handle) == 0 {
		return nil, errors.Errorf("unable to load items authors")
	}
	col := make(app.ItemCollection, len(items))
	authors, err := r.LoadAccounts(fActors)
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

func (r *repository) LoadItems(f app.LoadItemsFilter) (app.ItemCollection, error) {
	var qs string
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	target := "self"
	c := "outbox"
	if len(f.FollowedBy) > 0 {
		for _, foll := range f.FollowedBy {
			target = fmt.Sprintf("actors/%s", foll)
			c = "inbox"
			break
		}
	}

	if len(f.Federated) > 0 {
		for _, fed := range f.Federated {
			if fed {
				c = "inbox"
			}
		}
	}
	url := fmt.Sprintf("%s/%s/%s%s", r.BaseURL, target, c, qs)

	var err error
	var resp *http.Response
	ctx := log.Ctx{
		"url": url,
	}
	if resp, err = r.client.Get(url); err != nil {
		r.logger.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	ctx["code"] = resp.StatusCode
	ctx["status"] = resp.Status
	if resp == nil {
		err := fmt.Errorf("nil response from the repository")
		r.logger.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.New("unable to load from the API")
		r.logger.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	defer resp.Body.Close()
	var body []byte

	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return nil, err
	}
	col := ap.OrderedCollectionNew(as.ObjectID(url))
	if err = j.Unmarshal(body, &col); err != nil {
		r.logger.Error(err.Error())
		return nil, err
	}

	items := make(app.ItemCollection, col.TotalItems)
	for k, it := range col.OrderedItems {
		i := app.Item{}
		if err := i.FromActivityPub(it); err != nil {
			r.logger.Error(err.Error())
			continue
		}
		items[k] = i
	}
	return r.loadItemsAuthors(items...)
}

func (r *repository) SaveVote(v app.Vote) (app.Vote, error) {
	url := fmt.Sprintf("%s/actors/%s/liked/%s", r.BaseURL, v.SubmittedBy.Hash, v.Item.Hash)

	var err error
	var exists *http.Response
	// first step is to verify if vote already exists:
	if exists, err = r.client.Head(url); err != nil {
		r.logger.WithContext(log.Ctx{
			"url":   url,
			"err":   err,
			"trace": errors.Details(err),
		}).Error(err.Error())
		return v, err
	}
	p := loadAPPerson(*v.SubmittedBy)
	o := loadAPItem(*v.Item)
	id := as.ObjectID(url)

	var act ap.Activity
	act.ID = id
	act.Actor = p.GetLink()
	act.Object = o.GetLink()

	if v.Weight > 0 {
		act.Type = as.LikeType
	} else {
		act.Type = as.DislikeType
	}

	var body []byte
	if body, err = j.Marshal(act); err != nil {
		r.logger.Error(err.Error())
		return v, err
	}

	var resp *http.Response
	if exists.StatusCode == http.StatusOK {
		// previously found a vote, needs updating
		resp, err = r.client.Put(url, "application/json+activity", bytes.NewReader(body))
	} else if exists.StatusCode == http.StatusNotFound {
		// previously didn't fund a vote, needs adding
		resp, err = r.client.Post(url, "application/json+activity", bytes.NewReader(body))
	} else {
		err = errors.New("received unexpected http response")
		r.logger.WithContext(log.Ctx{
			"url":           url,
			"response_code": exists.StatusCode,
			"trace":         errors.Details(err),
		}).Error(err.Error())
		return v, err
	}

	if err != nil {
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

func (r *repository) LoadVotes(f app.LoadVotesFilter) (app.VoteCollection, error) {
	var qs string
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	var err error
	var resp *http.Response
	url := fmt.Sprintf("%s/self/liked%s", r.BaseURL, qs)
	if resp, err = r.client.Get(url); err != nil {
		r.logger.Error(err.Error())
		return nil, err
	}
	if resp == nil {
		r.logger.Error(err.Error())
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.New("unable to load from the API")
		r.logger.Error(err.Error())
		return nil, err
	}

	var items app.VoteCollection
	defer resp.Body.Close()
	var body []byte

	col := ap.OrderedCollectionNew(as.ObjectID(url))
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, err
	}
	if err := j.Unmarshal(body, &col); err != nil {
		return nil, err
	}
	items = make(app.VoteCollection, col.TotalItems)
	for k, it := range col.OrderedItems {
		vot := app.Vote{}
		if err := vot.FromActivityPub(it); err != nil {
			r.logger.Warn(err.Error())
			continue
		}
		items[k] = vot
	}
	return items, nil
}

func (r *repository) LoadVote(f app.LoadVotesFilter) (app.Vote, error) {
	if len(f.ItemKey) == 0 {
		return app.Vote{}, errors.New("invalid item hash")
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
		err := errors.New("nil response from the repository")
		r.logger.Error(err.Error())
		return v, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.New("unable to load from the API")
		r.logger.Error(err.Error())
		return v, err
	}
	defer resp.Body.Close()

	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return v, err
	}

	var like ap.Activity
	if err := j.Unmarshal(body, &like); err != nil {
		r.logger.Error(err.Error())
		return v, err
	}
	err = v.FromActivityPub(like)
	return v, err
}

func (r *repository) SaveItem(it app.Item) (app.Item, error) {
	doUpd := false
	art := loadAPItem(it)

	actor := loadAPPerson(frontend.AnonymousAccount())
	if r.Account != nil && r.Account.Hash == it.SubmittedBy.Hash {
		// need to test if it.SubmittedBy matches r.Account and that the signature is valid
		actor = loadAPPerson(*it.SubmittedBy)
	}

	var body []byte
	var err error
	if len(*art.GetID()) == 0 {
		id := as.ObjectID("")
		create := as.CreateNew(id, art)
		create.Actor = actor.GetLink()
		body, err = j.Marshal(create)
	} else {
		id := art.GetID()
		doUpd = true
		update := as.UpdateNew(*id, art)
		update.Actor = actor.GetLink()
		body, err = j.Marshal(update)
	}

	if err != nil {
		r.logger.WithContext(log.Ctx{
			"item":  it.Hash,
			"trace": errors.Details(err),
		}).Error(err.Error())
		return it, err
	}
	var resp *http.Response
	if doUpd {
		url := string(*art.GetID())
		resp, err = r.client.Put(url, "application/activity+json", bytes.NewReader(body))
	} else {
		url := fmt.Sprintf("%s/actors/%s/outbox", r.BaseURL, it.SubmittedBy.Hash)
		//url := fmt.Sprintf("%s/self/outbox", r.BaseURL)
		resp, err = r.client.Post(url, "application/activity+json", bytes.NewReader(body))
	}
	if err != nil {
		r.logger.Error(err.Error())
		return it, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		if a, ok := art.(ap.Article); ok {
			err := it.FromActivityPub(a)
			return it, err
		} else {
			hash := path.Base(string(*a.GetID()))
			filt := app.LoadItemsFilter{
				Key: []string{hash},
			}
			return r.LoadItem(filt)
		}
	case http.StatusCreated:
		newLoc := resp.Header.Get("Location")
		hash := path.Base(newLoc)
		f := app.LoadItemsFilter{
			Key: []string{hash},
		}
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

func (r *repository) LoadAccounts(f app.LoadAccountsFilter) (app.AccountCollection, error) {
	var qs string
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}
	url := fmt.Sprintf("%s/actors%s", r.BaseURL, qs)

	var err error
	var resp *http.Response
	if resp, err = r.client.Get(url); err != nil {
		r.logger.Error(err.Error())
		return nil, err
	}
	if resp == nil {
		err := fmt.Errorf("nil response from the repository")
		r.logger.Error(err.Error())
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.New("unable to load from the API")
		r.logger.Error(err.Error())
		return nil, err
	}
	defer resp.Body.Close()
	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return nil, err
	}
	col := ap.CollectionNew(as.ObjectID(url))
	if err = j.Unmarshal(body, &col); err != nil {
		r.logger.Error(err.Error())
		return nil, err
	}
	accounts := make(app.AccountCollection, 0)
	for _, it := range col.Items {
		acc := app.Account{}
		if err := acc.FromActivityPub(it); err != nil {
			r.logger.WithContext(log.Ctx{
				"type":  fmt.Sprintf("%T", it),
				"trace": errors.Details(err),
			}).Warn(err.Error())
			continue
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func (r *repository) LoadAccount(f app.LoadAccountsFilter) (app.Account, error) {
	var accounts app.AccountCollection
	var err error
	if accounts, err = r.LoadAccounts(f); err != nil {
		return app.Account{}, err
	}
	var ac *app.Account
	if ac, err = accounts.First(); err != nil {
		return app.Account{}, err
	}
	return *ac, nil
}

func (r *repository) SaveAccount(a app.Account) (app.Account, error) {
	return db.Config.SaveAccount(a)
}

type SignFunc func(r *http.Request) error

func getSigner(pubKeyID as.ObjectID, key crypto.PrivateKey) *httpsig.Signer {
	hdrs := []string{"(request-target)", "host", "test", "date"}
	return httpsig.NewSigner(string(pubKeyID), key, httpsig.RSASHA256, hdrs)
}

func (r *repository) LoadInfo() (app.Info, error) {
	inf := app.Info{}

	url := fmt.Sprintf("%s/self", r.BaseURL)
	var err error
	var resp *http.Response
	if resp, err = r.client.Get(url); err != nil {
		r.logger.Error(err.Error())
		return inf, err
	}
	if resp == nil {
		err := fmt.Errorf("nil response from the repository")
		r.logger.Error(err.Error())
		return inf, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.New("unable to load from the API")
		r.logger.Error(err.Error())
		return inf, err
	}
	defer resp.Body.Close()
	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return inf, err
	}
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return inf, err
	}

	s := as.Service{}
	if err = j.Unmarshal(body, &s); err != nil {
		r.logger.Error(err.Error())
		return inf, err
	}

	inf.Title = s.Name.First()
	inf.Summary = s.Summary.First()
	inf.Description = s.Content.First()
	inf.Languages = []string{"en"}

	inf.Email = fmt.Sprintf("%s@%s", "system", app.Instance.HostName)
	inf.Version = app.Instance.Version

	return inf, nil
}
