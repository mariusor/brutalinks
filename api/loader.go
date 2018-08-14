package api

import (
	"context"
	"fmt"
	"github.com/go-chi/chi"
	"github.com/juju/errors"
	ap "github.com/mariusor/activitypub.go/activitypub"
	j "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

const ServiceCtxtKey = "__loader"
const AccountCtxtKey = "__acct"
const CollectionCtxtKey = "__collection"
const FilterCtxtKey = "__filter"
const ItemCtxtKey = "__item"
const VotesCtxtKey = "__votes"
const VoteCtxtKey = "__vote"

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
			log.Infof("loaded LoaderService of type %T", AcctLoader)
		} else {
			log.Errorf("could not load account loader service from Context")
		}
		a, err := AcctLoader.LoadAccount(models.LoadAccountFilter{Handle: handle})
		if err != nil {
			log.Error(err)
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
				log.Errorf("could not load item filter from Context")
			}
			loader, ok := val.(models.CanLoadItems)
			if !ok {
				log.Errorf("could not load item loader service from Context")
			}
			i, err = loader.LoadItem(filters)
			if err != nil {
				log.Error(err)
			}
		}
		if col == "liked" {
			filters, ok := f.(models.LoadVotesFilter)
			if !ok {
				log.Errorf("could not load vote filter from Context")
			}
			loader, ok := val.(models.CanLoadVotes)
			if !ok {
				log.Errorf("could not load votes loader service from Context")
			}
			i, err = loader.LoadVote(filters)
			if err != nil {
				log.Error(err)
			}
		}

		ctx := context.WithValue(r.Context(), ItemCtxtKey, i)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func loadOutboxFilterFromReq(r *http.Request) models.LoadItemsFilter {
	filters := models.LoadItemsFilter{
		MaxItems:    app.MaxContentItems,
	}

	val := r.Context().Value(AccountCtxtKey)
	a, ok := val.(models.Account)
	if ok {
		filters.SubmittedBy = []string{a.Hash}
	}
	hash := chi.URLParam(r, "hash")
	if len(hash) > 0 {
		filters.Key = []string{hash}
		filters.MaxItems = 1
	}

	return filters
}

func loadLikedFilterFromReq(r *http.Request) models.LoadVotesFilter {
	types := r.URL.Query()["type"]
	var which []models.VoteType
	if types == nil {
		which = []models.VoteType{
			models.VoteType(strings.ToLower(string(ap.LikeType))),
			models.VoteType(strings.ToLower(string(ap.DislikeType))),
		}
	} else {
		for _, typ := range types {
			if strings.ToLower(typ) == strings.ToLower(string(ap.LikeType)) {
				which = []models.VoteType{models.VoteType(strings.ToLower(string(ap.LikeType)))}
			} else {
				which = []models.VoteType{models.VoteType(strings.ToLower(string(ap.DislikeType)))}
			}
		}
	}

	filters := models.LoadVotesFilter{
		Type:        which,
		MaxItems:    app.MaxContentItems,
	}
	val := r.Context().Value(AccountCtxtKey)
	a, ok := val.(models.Account)
	if ok {
		filters.SubmittedBy = []string{a.Hash}
	}
	hash := chi.URLParam(r, "hash")
	if len(hash) > 0 {
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
				log.Errorf("could not load item filters from Context")
			}
			loader, ok := val.(models.CanLoadItems)
			if !ok {
				log.Errorf("could not load item loader service from Context")
			}
			items, err = loader.LoadItems(filters)
			if err != nil {
				log.Error(err)
			}
		}
		if col == "liked" {
			filters, ok := f.(models.LoadVotesFilter)
			if !ok {
				log.Errorf("could not load votes filters from Context")
			}
			loader, ok := val.(models.CanLoadVotes)
			if !ok {
				log.Errorf("could not load votes loader service from Context")
			}
			items, err = loader.LoadVotes(filters)
			if err != nil {
				log.Error(err)
			}
		}

		ctx := context.WithValue(r.Context(), CollectionCtxtKey, items)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func (l LoaderService) LoadItem(f models.LoadItemsFilter) (models.Item, error) {
	return models.Item{}, errors.Errorf("not implemented")
}

func (l LoaderService) LoadItems(f models.LoadItemsFilter) (models.ItemCollection, error) {
	return LoadItems(f)
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

func LoadItems(f models.LoadItemsFilter) (models.ItemCollection, error) {
	apiBaseUrl := os.Getenv("LISTEN")

	var err error
	resp, err := http.Get(fmt.Sprintf("http://localhost%s/api/outbox", apiBaseUrl))
	if err != nil {
		log.Error(err)
		return nil, err
	}
	col := OrderedCollection{}
	if resp != nil {
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Error(err)
			return nil, err
		}
		err = j.Unmarshal(body, &col)
		if err != nil {
			log.Error(err)
			return nil, err
		}
	}

	items := make(models.ItemCollection, col.TotalItems)
	for k, it := range col.OrderedItems {
		c := models.Item{
			Hash:        getHash(it.GetID()),
			Title:       string(ap.NaturalLanguageValue(it.Name).First()),
			MimeType:    string(it.MediaType),
			Data:        string(ap.NaturalLanguageValue(it.Content).First()),
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
		items[k] = c
	}

	return items, nil
}
