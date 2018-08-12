package api

import (
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/models"
	"fmt"
	"os"
	"context"
	"net/http"
	"io/ioutil"
	ap "github.com/mariusor/activitypub.go/activitypub"
	j "github.com/mariusor/activitypub.go/jsonld"
	log "github.com/sirupsen/logrus"
)

const ServiceCtxtKey = "__loader"

// Service is used to retrieve information from the database
var Service LoaderService

type LoaderService struct {
	BaseUrl string
}

// Loader middleware
func Loader (next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ServiceCtxtKey, Service)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func (l LoaderService) LoadItem(f models.LoadItemFilter) (models.Item, error) {
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
			Hash: getHash(it.GetID()),
			Title: string(ap.NaturalLanguageValue(it.Name).First()),
			MimeType: string(it.MediaType),
			Data: string(ap.NaturalLanguageValue(it.Content).First()),
			Score: it.Score,
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

