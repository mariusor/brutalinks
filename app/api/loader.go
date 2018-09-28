package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/mariusor/qstring"

	"github.com/buger/jsonparser"
	"github.com/go-chi/chi"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	j "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

const RepositoryCtxtKey = "__repository"
const AccountCtxtKey = "__acct"
const CollectionCtxtKey = "__collection"
const FilterCtxtKey = "__filter"
const ItemCtxtKey = "__item"

// Config is used to retrieve information from the database
var Config repository

type repository struct {
	BaseUrl string
}

func (r repository) Head(url string) (resp *http.Response, err error) {
	return http.Head(url)
}

func (r repository) Get(url string) (resp *http.Response, err error) {
	return http.Get(url)
}

func (r repository) Post(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	return http.Post(url, contentType, body)
}

func (r repository) Put(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := http.NewRequest(http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

func (r repository) Delete(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := http.NewRequest(http.MethodDelete, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

// Repository middleware
func Repository(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), RepositoryCtxtKey, Config)
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
		val := r.Context().Value(RepositoryCtxtKey)
		AcctLoader, ok := val.(models.CanLoadAccounts)
		if !ok {
			log.WithFields(log.Fields{}).Errorf("could not load account repository from Context")
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

		f := r.Context().Value(FilterCtxtKey)
		val := r.Context().Value(RepositoryCtxtKey)

		var err error
		var i interface{}
		if col == "outbox" {
			filters, ok := f.(models.LoadItemsFilter)
			if !ok {
				log.WithFields(log.Fields{}).Errorf("could not load item filter from Context")
			}
			loader, ok := val.(models.CanLoadItems)
			if !ok {
				log.WithFields(log.Fields{}).Errorf("could not load item repository from Context")
				HandleError(w, r, http.StatusInternalServerError, err)
				return
			}
			i, err = loader.LoadItem(filters)
			if err != nil {
				log.WithFields(log.Fields{}).Error(err)
				HandleError(w, r, http.StatusNotFound, err)
				return
			}
		}
		if col == "liked" {
			filters, ok := f.(models.LoadVotesFilter)
			if !ok {
				log.WithFields(log.Fields{}).Errorf("could not load vote filter from Context")
			}
			loader, ok := val.(models.CanLoadVotes)
			if !ok {
				log.WithFields(log.Fields{}).Errorf("could not load vote repository from Context")
				HandleError(w, r, http.StatusInternalServerError, err)
				return
			}
			i, err = loader.LoadVote(filters)
			if err != nil {
				log.WithFields(log.Fields{}).Error(err)
				HandleError(w, r, http.StatusNotFound, err)
				return
			}
		}

		ctx := context.WithValue(r.Context(), ItemCtxtKey, i)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
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

		ctx := context.WithValue(r.Context(), FilterCtxtKey, filters)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func ItemCollectionCtxt(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		col := chi.URLParam(r, "collection")
		var err error

		f := r.Context().Value(FilterCtxtKey)
		val := r.Context().Value(RepositoryCtxtKey)

		var items interface{}
		if col == "outbox" {
			filters, ok := f.(models.LoadItemsFilter)
			if !ok {
				log.WithFields(log.Fields{}).Errorf("could not load item filters from Context")
			}
			loader, ok := val.(models.CanLoadItems)
			if !ok {
				log.WithFields(log.Fields{}).Errorf("could not load item repository from Context")
				HandleError(w, r, http.StatusInternalServerError, err)
				return
			}
			items, err = loader.LoadItems(filters)
			if err != nil {
				log.WithFields(log.Fields{}).Error(err)
				HandleError(w, r, http.StatusNotFound, err)
				return
			}
		}
		if col == "liked" {
			filters, ok := f.(models.LoadVotesFilter)
			if !ok {
				log.WithFields(log.Fields{}).Errorf("could not load votes filters from Context")
			}
			loader, ok := val.(models.CanLoadVotes)
			if !ok {
				log.WithFields(log.Fields{}).Errorf("could not load vote repository from Context")
				HandleError(w, r, http.StatusInternalServerError, err)
				return
			}
			items, err = loader.LoadVotes(filters)
			if err != nil {
				log.WithFields(log.Fields{}).Error(err)
				HandleError(w, r, http.StatusNotFound, err)
				return
			}
		}

		ctx := context.WithValue(r.Context(), CollectionCtxtKey, items)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func (r repository) LoadItem(f models.LoadItemsFilter) (models.Item, error) {
	var art Article
	var it models.Item
	var err error

	f.MaxItems = 1
	f.AttributedTo = nil
	hashes := f.Key
	f.Key = nil
	qs := ""

	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}
	url := fmt.Sprintf("http://%s/api/outbox/%s%s", r.BaseUrl, hashes[0], qs)
	resp, err := r.Get(url)
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
		return it, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			err := fmt.Errorf("unable to load from the API")
			log.WithFields(log.Fields{}).Error(err)
			return it, err
		}
		defer resp.Body.Close()

		if body, err := ioutil.ReadAll(resp.Body); err == nil {
			if err := j.Unmarshal(body, &art); err == nil {
				return loadFromAPItem(art)
			}
		}
	}
	log.WithFields(log.Fields{}).Error(err)
	return it, err
}

func (r repository) LoadItems(f models.LoadItemsFilter) (models.ItemCollection, error) {
	qs := ""
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	var err error
	resp, err := r.Get(fmt.Sprintf("http://%s/api/outbox%s", r.BaseUrl, qs))
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	col := OrderedCollection{}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			err := fmt.Errorf("unable to load from the API")
			log.WithFields(log.Fields{}).Error(err)
			return nil, err
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.WithFields(log.Fields{}).Error(err)
			return nil, err
		}
		err = j.Unmarshal(body, &col)
		if err != nil {
			log.WithFields(log.Fields{}).Error(err)
			return nil, err
		}
	}

	items := make(models.ItemCollection, col.TotalItems)
	for k, it := range col.OrderedItems {
		if art, ok := it.(*Article); ok {
			items[k], _ = loadFromAPItem(*art)
		} else {
			log.WithFields(log.Fields{}).Errorf("unable to load Article from %T", it)
		}
	}

	return items, nil
}

func (r repository) SaveVote(v models.Vote) (models.Vote, error) {
	//body := nil
	url := fmt.Sprintf("http://%s/api/accounts/%s/liked/%s", r.BaseUrl, v.SubmittedBy.Hash, v.Item.Hash)

	// first step is to verify if vote already exists:
	//resp, err := r.Head(url)
	//if err != nil {
	//	log.WithFields(log.Fields{}).Error(err)
	//	return v, err
	//}
	//var exists bool
	//if resp.StatusCode == http.StatusOK {
	//	// found a vote, needs updating
	//	exists = true
	//}
	//if resp.StatusCode == http.StatusNotFound {
	//	// found a vote, needs updating
	//	exists = false
	//}
	p := loadAPPerson(*v.SubmittedBy)
	o, err := loadAPItem(*v.Item)
	id := ap.ObjectID(url)
	var body []byte
	var act ap.Activity
	if v.Weight > 0 {
		like := ap.LikeActivityNew(id, ap.IRI(p.ID), ap.IRI(*o.GetID()))
		body, err = j.Marshal(like)
		act = ap.Activity(*like.Activity)
	} else {
		like := ap.DislikeActivityNew(id, ap.IRI(p.ID), ap.IRI(*o.GetID()))
		body, err = j.Marshal(like)
		act = ap.Activity(*like.Activity)
	}
	if err != nil {
		log.Error(err)
		return v, err
	}

	resp, err := r.Put(url, "application/json+activity", bytes.NewReader(body))
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
		return v, err
	}
	if resp.StatusCode == http.StatusOK {
		return loadFromAPLike(act)
	}
	if resp.StatusCode == http.StatusNotFound {
		return models.Vote{}, errors.Errorf("not found")
	}
	if resp.StatusCode == http.StatusInternalServerError {
		return models.Vote{}, errors.Errorf("unable to save vote")
	}
	return models.Vote{}, errors.Errorf("unknown error, received status %d", resp.StatusCode)
}

func (r repository) LoadVotes(f models.LoadVotesFilter) (models.VoteCollection, error) {
	qs := ""

	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}
	url := fmt.Sprintf("http://%s/api/liked%s", r.BaseUrl, qs)
	resp, err := r.Get(url)
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
		return nil, err
	}

	var items models.VoteCollection
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			err := fmt.Errorf("unable to load from the API")
			log.WithFields(log.Fields{}).Error(err)
			return nil, err
		}
		defer resp.Body.Close()

		col := OrderedCollection{}
		if body, err := ioutil.ReadAll(resp.Body); err == nil {
			if err := j.Unmarshal(body, &col); err == nil {
				items = make(models.VoteCollection, col.TotalItems)
				for _, it := range col.OrderedItems {
					if like, ok := it.(*ap.Activity); ok {
						vot, _ := loadFromAPLike(*like)
						items[vot.Item.Hash] = vot
					} else {
						log.WithFields(log.Fields{}).Errorf("unable to load Activity from %T", it)
					}
				}
			}
		}
	}
	return items, nil
}

func (r repository) LoadVote(f models.LoadVotesFilter) (models.Vote, error) {
	return db.Config.LoadVote(f)
}

func (r repository) SaveItem(it models.Item) (models.Item, error) {
	return it, errors.Errorf("not implemented")
}

func jsonUnescape(s string) string {
	if out, err := jsonparser.Unescape([]byte(s), nil); err != nil {
		log.WithFields(log.Fields{}).Error(err)
		return s
	} else {
		return string(out)
	}
}

func loadFromAPItem(it Article) (models.Item, error) {
	title := jsonUnescape(ap.NaturalLanguageValue(it.Name).First())
	content := jsonUnescape(ap.NaturalLanguageValue(it.Content).First())

	c := models.Item{
		Hash:        getHash(it.GetID()),
		Title:       title,
		MimeType:    string(it.MediaType),
		Data:        content,
		Score:       it.Score,
		SubmittedAt: it.Published,
		SubmittedBy: &models.Account{
			Handle: getAccountHandle(it.AttributedTo),
		},
	}
	r := it.InReplyTo
	if p, ok := r.(ap.IRI); ok {
		c.Parent = &models.Item{
			Hash: models.Hash(getAccountHandle(p)),
		}
	}
	if it.Context != it.InReplyTo {
		op := it.Context
		if p, ok := op.(ap.IRI); ok {
			c.OP = &models.Item{
				Hash: models.Hash(getAccountHandle(p)),
			}
		}
	}
	return c, nil
}

func loadFromAPLike(l ap.Activity) (models.Vote, error) {
	v := models.Vote{
		Flags: 0,
	}
	if l.Object != nil {
		if l.Object.IsLink() {
			i := ap.ObjectID(l.Object.(ap.IRI))
			v.Item = &models.Item{
				Hash: getHash(&i),
			}
		}
		if l.Object.IsObject() {
			v.Item = &models.Item{
				Hash: getHash(l.Object.GetID()),
			}
		}
	}
	if l.AttributedTo != nil {
		if l.AttributedTo.IsLink() {
			i := ap.ObjectID(l.AttributedTo.(ap.IRI))
			v.SubmittedBy = &models.Account{
				Hash: models.Hash(getHash(&i)),
			}
		}
		if l.AttributedTo.IsObject() {
			v.SubmittedBy = &models.Account{
				Hash: models.Hash(getHash(l.AttributedTo.GetID())),
			}
		}
	}
	//CreatedAt: nil,
	//UpdatedAt: nil,
	if l.Type == ap.LikeType {
		v.Weight = 1
	}
	if l.Type == ap.DislikeType {
		v.Weight = -1
	}
	return v, nil
}

func loadFromAPPerson(p Person) (models.Account, error) {
	name := jsonUnescape(ap.NaturalLanguageValue(p.Name).First())
	a := models.Account{
		Hash:   models.Hash(getHash(p.GetID())),
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
		Flags: 0,
		Votes: nil,
	}
	return a, nil
}

func (r repository) LoadAccounts(f models.LoadAccountsFilter) (models.AccountCollection, error) {
	qs := ""
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	accounts := make(models.AccountCollection, 0)
	resp, err := r.Get(fmt.Sprintf("http://%s/api/accounts?%s", r.BaseUrl, qs))
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	if resp != nil {
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		err = j.Unmarshal(body, &accounts)
		if err != nil {
			return nil, err
		}
	}
	return nil, errors.Errorf("not implemented")
}

func (r repository) LoadAccount(f models.LoadAccountsFilter) (models.Account, error) {
	p := Person{}

	if len(f.Handle) == 0 {
		return models.Account{}, errors.New("invalid account handle")
	}
	handle := f.Handle[0]
	resp, err := r.Get(fmt.Sprintf("http://%s/api/accounts/%s", r.BaseUrl, handle))
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
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

func (r repository) SaveAccount(a models.Account) (models.Account, error) {
	return models.Account{}, errors.Errorf("not implemented")
}
