package api

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/dyninc/qstring"
	"github.com/go-chi/chi"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	j "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
)

const ServiceCtxtKey = "__loader"
const AccountCtxtKey = "__acct"
const CollectionCtxtKey = "__collection"
const FilterCtxtKey = "__filter"
const ItemCtxtKey = "__item"

// Service is used to retrieve information from the database
var Service LoaderService

type LoaderService struct {
	BaseUrl string
}

// Loader middleware
func Loader(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ServiceCtxtKey, Service)
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
		val := r.Context().Value(ServiceCtxtKey)
		AcctLoader, ok := val.(models.CanLoadAccounts)
		if ok {
			log.WithFields(log.Fields{}).Infof("loaded LoaderService of type %T", AcctLoader)
		} else {
			log.WithFields(log.Fields{}).Errorf("could not load account loader service from Context")
		}
		a, err := AcctLoader.LoadAccount(models.LoadAccountFilter{Handle: handle})
		if err != nil {
			log.WithFields(log.Fields{}).Error(err)
			HandleError(w, r, http.StatusNotFound, err)
			return
		}
		if a.Handle == "" {
			HandleError(w, r, http.StatusNotFound, errors.Errorf("account not found"))
			return
		}

		ctx := context.WithValue(r.Context(), AccountCtxtKey, a)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func ItemCtxt(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		col := chi.URLParam(r, "collection")

		f := r.Context().Value(FilterCtxtKey)
		val := r.Context().Value(ServiceCtxtKey)

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
			}
			i, err = loader.LoadItem(filters)
			if err != nil {
				log.WithFields(log.Fields{}).Error(err)
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
			}
			i, err = loader.LoadVote(filters)
			if err != nil {
				log.WithFields(log.Fields{}).Error(err)
			}
		}

		ctx := context.WithValue(r.Context(), ItemCtxtKey, i)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func loadOutboxFilterFromReq(r *http.Request) models.LoadItemsFilter {
	filters := models.LoadItemsFilter{
		MaxItems: MaxContentItems,
	}

	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return filters
	}

	val := r.Context().Value(AccountCtxtKey)
	a, ok := val.(models.Account)
	if ok {
		filters.AttributedTo = []string{a.Hash}
	}

	return filters
}

func loadLikedFilterFromReq(r *http.Request) models.LoadVotesFilter {
	filters := models.LoadVotesFilter{
		MaxItems: MaxContentItems,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return filters
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
		val := r.Context().Value(ServiceCtxtKey)

		var items interface{}
		if col == "outbox" {
			filters, ok := f.(models.LoadItemsFilter)
			if !ok {
				log.WithFields(log.Fields{}).Errorf("could not load item filters from Context")
			}
			loader, ok := val.(models.CanLoadItems)
			if !ok {
				log.WithFields(log.Fields{}).Errorf("could not load item loader service from Context")
			}
			items, err = loader.LoadItems(filters)
			if err != nil {
				log.WithFields(log.Fields{}).Error(err)
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
			}
			items, err = loader.LoadVotes(filters)
			if err != nil {
				log.WithFields(log.Fields{}).Error(err)
			}
		}

		ctx := context.WithValue(r.Context(), CollectionCtxtKey, items)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func (l LoaderService) LoadItem(f models.LoadItemsFilter) (models.Item, error) {
	var art Article
	var it models.Item
	var err error
	if len(f.Key) != 1 {
		return it, errors.Errorf("invalid item hash")
	}
	resp, err := http.Get(fmt.Sprintf("http://%s/api/outbox/%s", l.BaseUrl, f.Key[0]))
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
		return it, err
	}
	if resp != nil {
		defer resp.Body.Close()

		if body, err := ioutil.ReadAll(resp.Body); err == nil {
			if err := j.Unmarshal(body, &art); err == nil {
				return loadFromAPItem(art), nil
			}
		}
	}
	log.WithFields(log.Fields{}).Error(err)
	return it, err
}

func (l LoaderService) LoadItems(f models.LoadItemsFilter) (models.ItemCollection, error) {
	qs := ""
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	log.Infof("qs url query %s", qs)
	var err error
	resp, err := http.Get(fmt.Sprintf("http://%s/api/outbox%s", l.BaseUrl, qs))
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
		return nil, err
	}
	col := OrderedCollection{}
	if resp != nil {
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
		items[k] = loadFromAPItem(it)
	}

	return items, nil
}

func (l LoaderService) SaveVote(v models.Vote) (models.Vote, error) {
	return models.Vote{}, errors.Errorf("not implemented")
}

func (l LoaderService) LoadVotes(f models.LoadVotesFilter) (models.VoteCollection, error) {
	return nil, errors.Errorf("not implemented") //models.LoadItemsVotes(f.ItemKey[0])
}

func (l LoaderService) LoadVote(f models.LoadVotesFilter) (models.Vote, error) {
	return models.Vote{}, errors.Errorf("not implemented")
}

func (l LoaderService) SaveItem(it models.Item) (models.Item, error) {
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

func loadFromAPItem(it Article) models.Item {
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
	return c
}
