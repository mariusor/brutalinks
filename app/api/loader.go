package api

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strings"

	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/frontend"

	"github.com/mariusor/qstring"

	"github.com/buger/jsonparser"
	"github.com/go-chi/chi"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	as "github.com/mariusor/activitypub.go/activitystreams"
	j "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

const AccountCtxtKey = "__acct"
const CollectionCtxtKey = "__collection"
const ItemCtxtKey = "__item"

// Config is used to retrieve information from the database
var Config repository

type repository struct {
	BaseUrl string
	Account *models.Account
}

func (r *repository) WithAccount(a *models.Account) {
	r.Account = a
}

func (r *repository) req(method string, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s-%s", app.Instance.HostName, app.Instance.Version))
	err = r.sign(req)
	if err != nil {
		new := errors.Errorf("unable to sign request")
		Logger.WithFields(log.Fields{
			"account":  r.Account.Handle,
			"url":      req.URL,
			"method":   req.Method,
			"previous": err.Error(),
		}).Warn(new)
	}
	return req, nil
}

func (r *repository) sign(req *http.Request) error {
	if r.Account == nil || r.Account == frontend.AnonymousAccount() {
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

	return SignRequest(req, p, prv)
}

func (r repository) Head(url string) (resp *http.Response, err error) {
	req, err := r.req(http.MethodHead, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func (r repository) Get(url string) (resp *http.Response, err error) {
	req, err := r.req(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func (r *repository) Post(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := r.req(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

func (r repository) Put(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := r.req(http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

func (r repository) Delete(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := r.req(http.MethodDelete, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

// Repository middleware
func Repository(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), models.RepositoryCtxtKey, &Config)
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

func AccountCtxt(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		handle := chi.URLParam(r, "handle")
		val := r.Context().Value(models.RepositoryCtxtKey)
		AcctLoader, ok := val.(models.CanLoadAccounts)
		if !ok {
			Logger.WithFields(log.Fields{}).Errorf("could not load account repository from Context")
		}
		a, err := AcctLoader.LoadAccount(models.LoadAccountsFilter{Handle: []string{handle}})
		if err == nil {
			// we redirect to the Hash based account URL
			url := strings.Replace(r.RequestURI, a.Handle, a.Hash.String(), 1)
			http.Redirect(w, r, url, http.StatusSeeOther)
			return
		} else {
			a, err := AcctLoader.LoadAccount(models.LoadAccountsFilter{Key: []string{handle}})
			if err != nil {
				log.Error(err)
				HandleError(w, r, http.StatusNotFound, err)
				return
			}
			if a.Handle == "" && len(a.Hash) == 0 {
				HandleError(w, r, http.StatusNotFound, errors.Errorf("account not found"))
				return
			}
			ctx := context.WithValue(r.Context(), AccountCtxtKey, a)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
	return http.HandlerFunc(fn)
}

func ItemCtxt(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		col := chi.URLParam(r, "collection")

		f := r.Context().Value(models.FilterCtxtKey)
		val := r.Context().Value(models.RepositoryCtxtKey)

		var err error
		var i interface{}
		if col == "outbox" {
			filters, ok := f.(models.LoadItemsFilter)
			if !ok {
				Logger.WithFields(log.Fields{}).Errorf("could not load item filter from Context")
			}
			loader, ok := val.(models.CanLoadItems)
			if !ok {
				Logger.WithFields(log.Fields{}).Errorf("could not load item repository from Context")
				HandleError(w, r, http.StatusInternalServerError, err)
				return
			}
			i, err = loader.LoadItem(filters)
			if err != nil {
				Logger.WithFields(log.Fields{}).Error(err)
				HandleError(w, r, http.StatusNotFound, err)
				return
			}
		}
		if col == "liked" {
			filters, ok := f.(models.LoadVotesFilter)
			if !ok {
				Logger.WithFields(log.Fields{}).Errorf("could not load vote filter from Context")
			}
			loader, ok := val.(models.CanLoadVotes)
			if !ok {
				Logger.WithFields(log.Fields{}).Errorf("could not load vote repository from Context")
				HandleError(w, r, http.StatusInternalServerError, err)
				return
			}
			i, err = loader.LoadVote(filters)
			if err != nil {
				Logger.WithFields(log.Fields{}).Error(err)
				HandleError(w, r, http.StatusNotFound, err)
				return
			}
		}

		ctx := context.WithValue(r.Context(), ItemCtxtKey, i)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func jsonUnescape(s string) string {
	var out []byte
	var err error
	if out, err = jsonparser.Unescape([]byte(s), nil); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return s
	}
	return string(out)
}

func loadFromAPObject(ob as.Object) (models.Item, error) {
	title := jsonUnescape(as.NaturalLanguageValue(ob.Name).First())
	content := jsonUnescape(as.NaturalLanguageValue(ob.Content).First())

	c := models.Item{
		Hash:        getHashFromAP(ob),
		Title:       title,
		MimeType:    string(ob.MediaType),
		Data:        content,
		SubmittedAt: ob.Published,
		SubmittedBy: &models.Account{
			Handle: getAccountHandle(ob.AttributedTo),
		},
	}
	r := ob.InReplyTo
	if p, ok := r.(as.IRI); ok {
		c.Parent = &models.Item{
			Hash: getHashFromAP(p),
		}
	}
	if ob.Context != ob.InReplyTo {
		op := ob.Context
		if p, ok := op.(as.IRI); ok {
			c.OP = &models.Item{
				Hash: getHashFromAP(p),
			}
		}
	}
	return c, nil
}
func loadFromAPCreate(act Activity) (models.Item, error) {
	it, err := loadFromAPItem(act.Object)
	it.SubmittedBy.Hash = getHashFromAP(act.Actor)
	return it, err
}

func loadFromAPActivity(it as.Item) (interface{}, error) {
	if !as.ValidActivityType(it.GetType()) {
		return models.Item{}, errors.Errorf("invalid object type received %s", it.GetType())
	}
	switch it.GetType() {
	case as.CreateType:
		return loadFromAPCreate(*it.(*Activity))
	case as.LikeType:
		fallthrough
	case as.DislikeType:
		return loadFromAPLike(*it.(*Activity))
	}
	return models.Item{}, errors.Errorf("not implemented for type %s", it.GetType())
}

func loadFromAPItem(it as.Item) (models.Item, error) {
	if it == nil {
		return models.Item{}, errors.New("nil item received")
	}
	if it.IsLink() {
		return models.Item{}, errors.New("unable to load from IRI")
	}
	if art, ok := it.(*Article); ok {
		return loadFromAPArticle(*art)
	}
	if art, ok := it.(Article); ok {
		return loadFromAPArticle(art)
	}
	if ob, ok := it.(*as.Object); ok {
		return loadFromAPObject(*ob)
	}
	if ob, ok := it.(as.Object); ok {
		return loadFromAPObject(ob)
	}
	return models.Item{}, errors.New("invalid object type")
}

func loadFromAPArticle(a Article) (models.Item, error) {
	it, err := loadFromAPObject(a.Object)
	it.Score = a.Score
	return it, err
}

func loadFromAPLike(l Activity) (models.Vote, error) {
	v := models.Vote{
		Flags: 0,
	}
	if l.Object != nil {
		v.Item = &models.Item{
			Hash: getHashFromAP(l.Object),
		}
	}
	if l.AttributedTo != nil {
		v.SubmittedBy = &models.Account{
			Hash: getHashFromAP(l.AttributedTo),
		}
	}
	//CreatedAt: nil,
	//UpdatedAt: nil,
	if l.Type == as.LikeType {
		v.Weight = 1
	}
	if l.Type == as.DislikeType {
		v.Weight = -1
	}
	return v, nil
}

func loadFromAPPerson(p Person) (models.Account, error) {
	name := jsonUnescape(as.NaturalLanguageValue(p.Name).First())
	a := models.Account{
		Hash:   getHashFromAP(p),
		Handle: name,
		Email:  "",
		Metadata: &models.AccountMetadata{
			Key: &models.SSHKey{
				ID:     "",
				Public: []byte(p.PublicKey.PublicKeyPem),
			},
		},
		Score: p.Score,
		//CreatedAt: nil,
		//UpdatedAt: nil,
		Flags: models.FlagsNone,
		Votes: nil,
	}
	return a, nil
}

func loadPersonFiltersFromReq(r *http.Request) models.LoadAccountsFilter {
	filters := models.LoadAccountsFilter{
		MaxItems: MaxContentItems,
	}

	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return filters
	}
	return filters
}

func loadOutboxFilterFromReq(r *http.Request) models.LoadItemsFilter {
	filters := models.LoadItemsFilter{
		MaxItems: MaxContentItems,
	}

	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return filters
	}
	handle := chi.URLParam(r, "handle")
	if handle != "" {
		old := filters.AttributedTo
		filters.AttributedTo = nil
		filters.AttributedTo = append(filters.AttributedTo, models.Hash(handle))
		filters.AttributedTo = append(filters.AttributedTo, old...)
	}
	hash := chi.URLParam(r, "hash")
	if hash != "" {
		old := filters.Key
		filters.Key = nil
		filters.Key = append(filters.Key, hash)
		filters.Key = append(filters.Key, old...)
	}

	//val := r.Context().Value(AccountCtxtKey)
	//a, ok := val.(models.Account)
	//if ok {
	//	filters.AttributedTo = []string{a.Hash}
	//}

	return filters
}

func loadLikedFilterFromReq(r *http.Request) models.LoadVotesFilter {
	filters := models.LoadVotesFilter{}
	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return filters
	}

	handle := chi.URLParam(r, "handle")
	if handle != "" {
		old := filters.AttributedTo
		filters.AttributedTo = nil
		filters.AttributedTo = append(filters.AttributedTo, models.Hash(handle))
		filters.AttributedTo = append(filters.AttributedTo, old...)
	}
	hash := chi.URLParam(r, "hash")
	if hash != "" {
		old := filters.ItemKey
		filters.ItemKey = nil
		filters.ItemKey = append(filters.ItemKey, hash)
		filters.ItemKey = append(filters.ItemKey, old...)
	}
	val := r.Context().Value(AccountCtxtKey)
	a, ok := val.(models.Account)
	if ok {
		filters.AttributedTo = []models.Hash{a.Hash}
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
		col := chi.URLParam(r, "collection")
		var filters interface{}
		switch col {
		case "disliked":
			fallthrough
		case "liked":
			filters = loadLikedFilterFromReq(r)
		case "outbox":
			filters = loadOutboxFilterFromReq(r)
		case "inbox":
		case "":
			filters = loadPersonFiltersFromReq(r)
		}

		ctx := context.WithValue(r.Context(), models.FilterCtxtKey, filters)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func ItemCollectionCtxt(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		col := chi.URLParam(r, "collection")
		var err error

		f := r.Context().Value(models.FilterCtxtKey)
		val := r.Context().Value(models.RepositoryCtxtKey)

		var items interface{}
		if col == "outbox" {
			filters, ok := f.(models.LoadItemsFilter)
			if !ok {
				Logger.WithFields(log.Fields{}).Errorf("could not load item filters from Context")
			}
			loader, ok := val.(models.CanLoadItems)
			if !ok {
				Logger.WithFields(log.Fields{}).Errorf("could not load item repository from Context")
				HandleError(w, r, http.StatusInternalServerError, err)
				return
			}
			items, err = loader.LoadItems(filters)
			if err != nil {
				Logger.WithFields(log.Fields{}).Error(err)
				HandleError(w, r, http.StatusNotFound, err)
				return
			}
		}
		if col == "liked" {
			filters, ok := f.(models.LoadVotesFilter)
			if !ok {
				Logger.WithFields(log.Fields{}).Errorf("could not load votes filters from Context")
			}
			loader, ok := val.(models.CanLoadVotes)
			if !ok {
				Logger.WithFields(log.Fields{}).Errorf("could not load vote repository from Context")
				HandleError(w, r, http.StatusInternalServerError, err)
				return
			}
			items, err = loader.LoadVotes(filters)
			if err != nil {
				Logger.WithFields(log.Fields{}).Error(err)
				HandleError(w, r, http.StatusNotFound, err)
				return
			}
		}

		ctx := context.WithValue(r.Context(), CollectionCtxtKey, items)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func (r *repository) LoadItem(f models.LoadItemsFilter) (models.Item, error) {
	var art Article
	var it models.Item

	f.MaxItems = 1
	f.AttributedTo = nil
	hashes := f.Key
	f.Key = nil

	var qs string
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}
	url := fmt.Sprintf("%s/outbox/%s/object%s", r.BaseUrl, hashes[0], qs)

	var err error
	var resp *http.Response
	if resp, err = r.Get(url); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return it, err
	}
	if resp == nil {
		err := fmt.Errorf("nil response from the repository")
		Logger.WithFields(log.Fields{}).Error(err)
		return it, err
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("unable to load from the API")
		Logger.WithFields(log.Fields{}).Error(err)
		return it, err
	}
	defer resp.Body.Close()
	var body []byte

	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return it, err
	}
	if err := j.Unmarshal(body, &art); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return it, err
	}
	return loadFromAPItem(art)
}

func (r *repository) LoadItems(f models.LoadItemsFilter) (models.ItemCollection, error) {
	var qs string
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}
	url := fmt.Sprintf("%s/outbox%s", r.BaseUrl, qs)

	var err error
	var resp *http.Response
	if resp, err = r.Get(url); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	if resp == nil {
		err := fmt.Errorf("nil response from the repository")
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("unable to load from the API")
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	defer resp.Body.Close()
	var body []byte

	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	col := OrderedCollectionNew(as.ObjectID(url))
	if err = j.Unmarshal(body, &col); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}

	items := make(models.ItemCollection, col.TotalItems)
	for k, it := range col.OrderedItems {
		obj, err := loadFromAPActivity(it)
		if err != nil {
			Logger.WithFields(log.Fields{}).Error(err)
			continue
		}
		if art, ok := obj.(models.Item); ok {
			items[k] = art
			continue
		}
		Logger.WithFields(log.Fields{}).Error(errors.Errorf("received invalid object "))
	}

	return items, nil
}

func (r *repository) SaveVote(v models.Vote) (models.Vote, error) {
	url := fmt.Sprintf("%s/accounts/%s/liked/%s", r.BaseUrl, v.SubmittedBy.Hash, v.Item.Hash)

	var err error
	var exists *http.Response
	// first step is to verify if vote already exists:
	if exists, err = r.Head(url); err != nil {
		Logger.WithFields(log.Fields{
			"url": url,
		}).Error(err)
		return v, err
	}
	p := loadAPPerson(*v.SubmittedBy)
	o := loadAPItem(*v.Item)
	id := as.ObjectID(url)

	var act Activity
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
		log.Error(err)
		return v, err
	}

	var resp *http.Response
	if exists.StatusCode == http.StatusOK {
		// previously found a vote, needs updating
		resp, err = r.Put(url, "application/json+activity", bytes.NewReader(body))
	} else if exists.StatusCode == http.StatusNotFound {
		// previously didn't fund a vote, needs adding
		resp, err = r.Post(url, "application/json+activity", bytes.NewReader(body))
	} else {
		err = errors.New("received unexpected http response")
		Logger.WithFields(log.Fields{
			"url":           url,
			"response_code": exists.StatusCode,
		}).Error(err)
		return v, err
	}

	if err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return v, err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return loadFromAPLike(act)
	}
	if resp.StatusCode == http.StatusNotFound {
		return models.Vote{}, errors.Errorf("vote not found")
	}
	if resp.StatusCode == http.StatusInternalServerError {
		return models.Vote{}, errors.Errorf("unable to save vote")
	}
	return models.Vote{}, errors.Errorf("unknown error, received status %d", resp.StatusCode)
}

func (r *repository) LoadVotes(f models.LoadVotesFilter) (models.VoteCollection, error) {
	var qs string
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	var err error
	var resp *http.Response
	url := fmt.Sprintf("%s/liked%s", r.BaseUrl, qs)
	if resp, err = r.Get(url); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	if resp == nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("unable to load from the API")
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}

	var items models.VoteCollection
	defer resp.Body.Close()
	var body []byte

	col := OrderedCollectionNew(as.ObjectID(url))
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, err
	}
	if err := j.Unmarshal(body, &col); err != nil {
		return nil, err
	}
	items = make(models.VoteCollection, col.TotalItems)
	for _, it := range col.OrderedItems {
		obj, err := loadFromAPActivity(it)
		if err != nil {
			Logger.WithFields(log.Fields{}).Error(err)
			continue
		}
		if vot, ok := obj.(models.Vote); ok {
			items[vot.Item.Hash] = vot
			continue
		}
		Logger.WithFields(log.Fields{}).Error(errors.Errorf("received invalid object "))
	}
	return items, nil
}

func (r *repository) LoadVote(f models.LoadVotesFilter) (models.Vote, error) {
	if len(f.ItemKey) == 0 {
		return models.Vote{}, errors.New("invalid item hash")
	}

	itemHash := f.ItemKey[0]
	f.ItemKey = nil
	url := fmt.Sprintf("%s/liked/%s", r.BaseUrl, itemHash)

	var err error
	var resp *http.Response
	if resp, err = r.Get(url); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return models.Vote{}, err
	}
	if resp == nil {
		err := errors.New("nil response from the repository")
		Logger.WithFields(log.Fields{}).Error(err)
		return models.Vote{}, err
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("unable to load from the API")
		Logger.WithFields(log.Fields{}).Error(err)
		return models.Vote{}, err
	}
	defer resp.Body.Close()

	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return models.Vote{}, err
	}

	var like Activity
	if err := j.Unmarshal(body, &like); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return models.Vote{}, err
	}
	return loadFromAPLike(like)
}

func (r *repository) SaveItem(it models.Item) (models.Item, error) {
	doUpd := false
	art := loadAPItem(it)

	actor := loadAPPerson(*frontend.AnonymousAccount())
	if r.Account != nil && r.Account.Hash == it.SubmittedBy.Hash {
		// need to test if it.SubmittedBy matches r.Account and that the signature is valid
		actor = loadAPPerson(*it.SubmittedBy)
	}

	var body []byte
	var err error
	if len(*art.GetID()) == 0 {
		id := as.ObjectID("")
		create := ap.CreateActivityNew(id, actor.GetLink(), art)
		body, err = j.Marshal(create)
	} else {
		id := art.GetID()
		doUpd = true
		update := ap.UpdateActivityNew(*id, actor.GetLink(), art)
		body, err = j.Marshal(update)
	}

	if err != nil {
		Logger.WithFields(log.Fields{"item": it.Hash}).Error(err)
		return it, err
	}
	var resp *http.Response
	if doUpd {
		url := string(*art.GetID())
		resp, err = r.Put(url, "application/json+activity", bytes.NewReader(body))
	} else {
		url := fmt.Sprintf("%s/accounts/%s/outbox", r.BaseUrl, it.SubmittedBy.Hash)
		resp, err = r.Post(url, "application/json+activity", bytes.NewReader(body))
	}
	if err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return it, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		if a, ok := art.(Article); ok {
			return loadFromAPItem(a)
		} else {
			hash := path.Base(string(*a.GetID()))
			filt := models.LoadItemsFilter{
				Key: []string{hash},
			}
			return r.LoadItem(filt)
		}
	case http.StatusCreated:
		newLoc := resp.Header.Get("Location")
		hash := path.Base(newLoc)
		f := models.LoadItemsFilter{
			Key: []string{hash},
		}
		return r.LoadItem(f)
	case http.StatusNotFound:
		return models.Item{}, errors.Errorf("%s", resp.Status)
	case http.StatusMethodNotAllowed:
		return models.Item{}, errors.Errorf("%s", resp.Status)
	case http.StatusInternalServerError:
		return models.Item{}, errors.Errorf("unable to save item %s", resp.Status)
	default:
		return models.Item{}, errors.Errorf("unknown error, received status %d", resp.StatusCode)
	}
}

func (r *repository) LoadAccounts(f models.LoadAccountsFilter) (models.AccountCollection, error) {
	var qs string
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}
	url := fmt.Sprintf("%s/accounts?%s", r.BaseUrl, qs)

	var err error
	var resp *http.Response
	if resp, err = r.Get(url); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	if resp == nil {
		err := fmt.Errorf("nil response from the repository")
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("unable to load from the API")
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	defer resp.Body.Close()
	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	col := OrderedCollectionNew(as.ObjectID(url))
	if err = j.Unmarshal(body, &col); err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	accounts := make(models.AccountCollection, 0)
	for k, it := range col.OrderedItems {
		var p Person
		var ok bool
		if p, ok = it.(Person); !ok {
			continue
		}
		accounts[k], _ = loadFromAPPerson(p)
	}
	return accounts, nil
}

func (r *repository) LoadAccount(f models.LoadAccountsFilter) (models.Account, error) {
	p := Person{}

	if len(f.Handle) == 0 {
		return models.Account{}, errors.New("invalid account handle")
	}
	handle := f.Handle[0]
	resp, err := r.Get(fmt.Sprintf("%s/accounts/%s", r.BaseUrl, handle))
	if err != nil {
		Logger.WithFields(log.Fields{}).Error(err)
		return models.Account{}, err
	}
	if resp != nil {
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return models.Account{}, err
		}
		err = j.Unmarshal(body, &p)
		if err != nil {
			return models.Account{}, err
		}
	}
	return loadFromAPPerson(p)
}

func (r *repository) SaveAccount(a models.Account) (models.Account, error) {
	return db.Config.SaveAccount(a)
}
