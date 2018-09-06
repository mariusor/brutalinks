package api

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/dyninc/qstring"

	"github.com/buger/jsonparser"
	"github.com/go-chi/chi"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	j "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/models"
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
		if ok {
			log.WithFields(log.Fields{}).Infof("loaded repository of type %T", AcctLoader)
		} else {
			log.WithFields(log.Fields{}).Errorf("could not load account loader service from Context")
		}
		a, err := AcctLoader.LoadAccount(models.LoadAccountFilter{Handle: handle})
		if err == nil {
			// we redirect to the Hash based account URL
			url := strings.Replace(r.RequestURI, a.Handle, a.Hash, 1)
			http.Redirect(w, r, url, http.StatusSeeOther)
			return
		} else {
			a, err := AcctLoader.LoadAccount(models.LoadAccountFilter{Key: handle})
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
				log.WithFields(log.Fields{}).Errorf("could not load item loader service from Context")
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
				log.WithFields(log.Fields{}).Errorf("could not load votes loader service from Context")
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
		filters.AttributedTo = append(filters.AttributedTo, handle)
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
	filters := models.LoadVotesFilter{
		MaxItems: MaxContentItems,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return filters
	}

	handle := chi.URLParam(r, "handle")
	if handle != "" {
		old := filters.AttributedTo
		filters.AttributedTo = nil
		filters.AttributedTo = append(filters.AttributedTo, handle)
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
		filters.AttributedTo = []string{a.Hash}
	}
	if len(filters.ItemKey) > 0 {
		filters.MaxItems = 1
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
				log.WithFields(log.Fields{}).Errorf("could not load item loader service from Context")
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
				log.WithFields(log.Fields{}).Errorf("could not load votes loader service from Context")
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

func (l repository) LoadItem(f models.LoadItemsFilter) (models.Item, error) {
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
	url := fmt.Sprintf("http://%s/api/outbox/%s%s", l.BaseUrl, hashes[0], qs)
	resp, err := http.Get(url)
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

func (l repository) LoadItems(f models.LoadItemsFilter) (models.ItemCollection, error) {
	qs := ""
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	var err error
	resp, err := http.Get(fmt.Sprintf("http://%s/api/outbox%s", l.BaseUrl, qs))
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
		items[k], _ = loadFromAPItem(it)
	}

	return items, nil
}

func (l repository) SaveVote(v models.Vote) (models.Vote, error) {
	//body := nil
	//url := fmt.Sprintf("http://%s/api/accounts/%s/liked/%s", l.BaseUrl, v.SubmittedBy.Hash, v.Item.Hash)
	//resp, err := http.Post(url, "application/json+activity", body)
	return models.Vote{}, errors.Errorf("not implemented")
}

func (l repository) LoadVotes(f models.LoadVotesFilter) (models.VoteCollection, error) {
	return nil, errors.Errorf("not implemented") //models.LoadItemsVotes(f.ItemKey[0])
}

func (l repository) LoadVote(f models.LoadVotesFilter) (models.Vote, error) {
	return models.Vote{}, errors.Errorf("not implemented")
}

func (l repository) SaveItem(it models.Item) (models.Item, error) {
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
			Hash: getAccountHandle(p),
		}
	}
	if it.Context != it.InReplyTo {
		op := it.Context
		if p, ok := op.(ap.IRI); ok {
			c.OP = &models.Item{
				Hash: getAccountHandle(p),
			}
		}
	}
	return c, nil
}

func loadFromAPPerson(p Person) (models.Account, error) {
	name := jsonUnescape(ap.NaturalLanguageValue(p.Name).First())
	a := models.Account{
		Hash:   getHash(p.GetID()),
		Handle: name,
		Email:  "",
		Metadata: &models.AccountMetadata{
			Key: &models.SSHKey{
				Id:     "",
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

func (l repository) LoadAccounts(f models.LoadAccountsFilter) (models.AccountCollection, error) {
	qs := ""
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	accounts := make(models.AccountCollection, 0)
	resp, err := http.Get(fmt.Sprintf("http://%s/api/accounts?%s", l.BaseUrl, qs))
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

func (l repository) LoadAccount(f models.LoadAccountFilter) (models.Account, error) {
	p := Person{}

	resp, err := http.Get(fmt.Sprintf("http://%s/api/accounts/%s", l.BaseUrl, f.Handle))
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

func (l repository) SaveAccount(a models.Account) (models.Account, error) {
	return models.Account{}, errors.Errorf("not implemented")
}
