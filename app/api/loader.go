package api

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"fmt"
	"github.com/writeas/go-nodeinfo"
	"io/ioutil"
	"net/http"
	"path"
	"strings"

	"github.com/spacemonkeygo/httpsig"

	cl "github.com/go-ap/activitypub/client"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/frontend"

	"github.com/mariusor/qstring"

	as "github.com/go-ap/activitystreams"
	j "github.com/go-ap/jsonld"
	"github.com/go-chi/chi"
	"github.com/mariusor/littr.go/internal/errors"
	ap "github.com/mariusor/littr.go/app/activitypub"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/internal/log"
)

type repository struct {
	BaseURL string
	Account *app.Account
	logger  log.Logger
	client  cl.HttpClient
}

func New(c Config) *repository {
	cl.UserAgent = fmt.Sprintf("%s-%s", app.Instance.HostName, app.Instance.Version)
	cl.ErrorLogger = func(el ...interface{}) { c.Logger.Errorf("%v", el) }
	cl.InfoLogger = func(el ...interface{}) { c.Logger.Infof("%v", el) }
	return &repository{
		BaseURL: c.BaseURL,
		logger:  c.Logger,
		client:  cl.NewClient(),
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
	s := getSigner(p.PublicKey.ID, prv)
	cl.Sign = s.Sign

	return nil
}

// Repository middleware
func (h handler) Repository(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), app.RepositoryCtxtKey, h.repo)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func (h handler) ServiceCtxt(next http.Handler) http.Handler {
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
			url := strings.Replace(r.RequestURI, handle, a.Hash.String(), 1)
			http.Redirect(w, r, url, http.StatusSeeOther)
			return
		} else {
			a, err := AcctLoader.LoadAccount(app.LoadAccountsFilter{Key: app.Hashes{app.Hash(handle)}})
			if err != nil {
				h.logger.Error(err.Error())
				h.HandleError(w, r, err)
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
			filters, ok := f.(*app.LoadItemsFilter)
			if !ok {
				h.logger.Error("could not load item filter from Context")
			}
			loader, ok := val.(app.CanLoadItems)
			if !ok {
				h.logger.Error("could not load item repository from Context")
				h.HandleError(w, r, errors.NewNotValid(err, "not found"))
				return
			}
			i, err = loader.LoadItem(*filters)
			if err != nil {
				h.logger.Error(err.Error())
				h.HandleError(w, r, errors.NewNotFound(err, "not found"))
				return
			}
		}
		if col == "liked" {
			filters, ok := f.(*app.LoadVotesFilter)
			if !ok {
				h.logger.Error("could not load vote filter from Context")
			}
			loader, ok := val.(app.CanLoadVotes)
			if !ok {
				h.logger.Error("could not load vote repository from Context")
				h.HandleError(w, r, errors.NewNotValid(err, "not found"))
				return
			}
			i, err = loader.LoadVote(*filters)
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

func loadPersonFiltersFromReq(r *http.Request) *app.LoadAccountsFilter {
	filters := app.LoadAccountsFilter{
		MaxItems: MaxContentItems,
	}

	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return &filters
	}
	return &filters
}

func loadRepliesFilterFromReq(r *http.Request) *app.LoadItemsFilter {
	filters := app.LoadItemsFilter{
		MaxItems: MaxContentItems,
	}
	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return &filters
	}
	hash := chi.URLParam(r, "hash")

	filters.InReplyTo = []string{hash}

	return &filters
}

func stringsUnique(a []string) []string {
	u := make([]string, 0, len(a))
	m := make(map[string]bool)

	for _, val := range a {
		if _, ok := m[val]; !ok {
			m[val] = true
			u = append(u, val)
		}
	}
	return u
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

func loadInboxFilterFromReq(r *http.Request) *app.LoadItemsFilter {
	filters := app.LoadItemsFilter{
		MaxItems: MaxContentItems,
	}

	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return &filters
	}
	handle := chi.URLParam(r, "handle")
	if handle != "" {
		old := filters.FollowedBy
		filters.FollowedBy = nil
		filters.FollowedBy = append(filters.FollowedBy, handle)
		filters.FollowedBy = append(filters.FollowedBy, old...)
		filters.FollowedBy = stringsUnique(filters.FollowedBy)
	}
	hash := app.Hash(chi.URLParam(r, "hash"))
	if hash != "" {
		old := filters.Key
		filters.Key = nil
		filters.Key = append(filters.Key, hash)
		filters.Key = append(filters.Key, old...)
		filters.Key = hashesUnique(filters.Key)
	}

	return &filters
}

func loadOutboxFilterFromReq(r *http.Request) *app.LoadItemsFilter {
	filters := app.LoadItemsFilter{
		MaxItems: MaxContentItems,
	}

	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return &filters
	}
	handle := chi.URLParam(r, "handle")
	if handle != "" {
		old := filters.AttributedTo
		filters.AttributedTo = nil
		filters.AttributedTo = append(filters.AttributedTo, app.Hash(handle))
		filters.AttributedTo = append(filters.AttributedTo, old...)
		filters.AttributedTo = hashesUnique(filters.AttributedTo)
	}
	hash := chi.URLParam(r, "hash")
	if hash != "" {
		old := filters.Key
		filters.Key = nil
		filters.Key = append(filters.Key, app.Hash(hash))
		filters.Key = append(filters.Key, old...)
		filters.Key = hashesUnique(filters.Key)
	}

	return &filters
}

func loadLikedFilterFromReq(r *http.Request) *app.LoadVotesFilter {
	filters := app.LoadVotesFilter{}
	if err := qstring.Unmarshal(r.URL.Query(), &filters); err != nil {
		return &filters
	}

	handle := chi.URLParam(r, "handle")
	if handle != "" {
		old := filters.AttributedTo
		filters.AttributedTo = nil
		filters.AttributedTo = append(filters.AttributedTo, app.Hash(handle))
		filters.AttributedTo = append(filters.AttributedTo, old...)
		filters.AttributedTo = hashesUnique(filters.AttributedTo)
	}
	hash := chi.URLParam(r, "hash")
	if hash != "" {
		old := filters.ItemKey
		filters.ItemKey = nil
		filters.ItemKey = append(filters.ItemKey, app.Hash(hash))
		filters.ItemKey = append(filters.ItemKey, old...)
		filters.ItemKey = hashesUnique(filters.ItemKey)
	}
	if filters.MaxItems == 0 {
		if len(filters.ItemKey) > 0 {
			filters.MaxItems = 1
		} else {
			filters.MaxItems = MaxContentItems
		}
	}
	return &filters
}

var validCollectionNames = []string{
	"actors",
	"liked",
	"outbox",
	"inbox",
	"replies",
	"followed",
	"following",
}

func isValidCollectionName(s string) bool {
	for _, valid := range validCollectionNames {
		if valid == s {
			return true
		}
	}
	return false
}

func LoadFiltersCtxt(eh app.ErrorHandler) app.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			col := getCollectionFromReq(r)
			var filters app.Paginator
			switch col {
			case "following":
				fallthrough
			case "actors":
				filters = loadPersonFiltersFromReq(r)
			case "liked":
				filters = loadLikedFilterFromReq(r)
			case "outbox":
				filters = loadOutboxFilterFromReq(r)
			case "inbox":
				filters = loadInboxFilterFromReq(r)
			case "replies":
				filters = loadRepliesFilterFromReq(r)
			case "":
			default:
				eh(w, r, errors.NotValidf("collection %s", col))
				return
			}

			ctx := context.WithValue(r.Context(), app.FilterCtxtKey, filters)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(fn)
	}
}

func (h handler) ItemCollectionCtxt(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		var err error
		var count uint
		col := getCollectionFromReq(r)

		f := r.Context().Value(app.FilterCtxtKey)
		val := r.Context().Value(app.RepositoryCtxtKey)

		var items interface{}
		switch col {
		case "inbox":
			fallthrough
		case "outbox":
			fallthrough
		case "replies":
			filters, ok := f.(*app.LoadItemsFilter)
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
			items, count, err = loader.LoadItems(*filters)
			if err != nil {
				h.logger.Error(err.Error())
				next.ServeHTTP(w, r)
				return
			}
		case "liked":
			filters, ok := f.(*app.LoadVotesFilter)
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
			items, count, err = loader.LoadVotes(*filters)
			if err != nil {
				h.logger.Error(err.Error())
				next.ServeHTTP(w, r)
				return
			}
		}

		ctx := context.WithValue(r.Context(), app.CollectionCtxtKey, items)
		ctx = context.WithValue(ctx, app.CollectionCountCtxtKey, count)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func (r *repository) LoadItem(f app.LoadItemsFilter) (app.Item, error) {
	var art ap.Article
	var it app.Item

	f.MaxItems = 1
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
		err := errors.New("empty response from the repository")
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
			fActors.Key = append(fActors.Key, it.SubmittedBy.Hash)
		} else if len(it.SubmittedBy.Handle) > 0 {
			fActors.Handle = append(fActors.Handle, it.SubmittedBy.Handle)
		}
	}
	fActors.Key = hashesUnique(fActors.Key)
	if len(fActors.Key)+len(fActors.Handle) == 0 {
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

func (r *repository) LoadItems(f app.LoadItemsFilter) (app.ItemCollection, uint, error) {
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
		return nil, 0, err
	}

	ctx["code"] = resp.StatusCode
	ctx["status"] = resp.Status
	if resp == nil {
		err := fmt.Errorf("nil response from the repository")
		r.logger.WithContext(ctx).Error(err.Error())
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.New("unable to load from the API")
		r.logger.WithContext(ctx).Error(err.Error())
		return nil, 0, err
	}
	defer resp.Body.Close()
	var body []byte

	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	col := ap.OrderedCollectionNew(as.ObjectID(url))
	if err = j.Unmarshal(body, &col); err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}

	count := col.TotalItems
	items := make(app.ItemCollection, 0)
	for _, it := range col.OrderedItems {
		i := app.Item{}
		if err := i.FromActivityPub(it); err != nil {
			r.logger.Error(err.Error())
			continue
		}
		items = append(items, i)
	}
	res, err := r.loadItemsAuthors(items...)
	return res, count, err
}

func (r *repository) SaveVote(v app.Vote) (app.Vote, error) {
	url := fmt.Sprintf("%s/actors/%s/outbox/%s", r.BaseURL, v.Item.SubmittedBy.Hash, v.Item.Hash)

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
	outbox := fmt.Sprintf("%s/actors/%s/outbox", r.BaseURL, v.Item.SubmittedBy.Hash)
	if exists.StatusCode == http.StatusOK {
		// previously found a vote, needs updating
		resp, err = r.client.Post(outbox, "application/json+activity", bytes.NewReader(body))
	} else if exists.StatusCode == http.StatusNotFound {
		// previously didn't fund a vote, needs adding
		resp, err = r.client.Post(outbox, "application/json+activity", bytes.NewReader(body))
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

func (r *repository) LoadVotes(f app.LoadVotesFilter) (app.VoteCollection, uint, error) {
	var qs string
	if q, err := qstring.MarshalString(&f); err == nil {
		qs = fmt.Sprintf("?%s", q)
	}

	var err error
	var resp *http.Response
	url := fmt.Sprintf("%s/self/liked%s", r.BaseURL, qs)
	if resp, err = r.client.Get(url); err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	if resp == nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.New("unable to load from the API")
		r.logger.Error(err.Error())
		return nil, 0, err
	}

	var votes app.VoteCollection
	defer resp.Body.Close()
	var body []byte

	col := ap.OrderedCollectionNew(as.ObjectID(url))
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, 0, err
	}
	if err := j.Unmarshal(body, &col); err != nil {
		return nil, 0, err
	}
	count := col.TotalItems
	votes = make(app.VoteCollection, 0)
	for _, it := range col.OrderedItems {
		vot := app.Vote{}
		if err := vot.FromActivityPub(it); err != nil {
			r.logger.Warn(err.Error())
			continue
		}
		votes = append(votes, vot)
	}
	return votes, count, nil
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
	art := loadAPItem(it)

	actor := loadAPPerson(frontend.AnonymousAccount())
	if r.Account != nil && r.Account.Hash == it.SubmittedBy.Hash {
		// need to test if it.SubmittedBy matches r.Account and that the signature is valid
		actor = loadAPPerson(*it.SubmittedBy)
	}

	var body []byte
	var err error
	if it.Deleted() {
		if len(*art.GetID()) == 0 {
			r.logger.WithContext(log.Ctx{
				"item":  it.Hash,
				"trace": errors.Details(err),
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
			"item":  it.Hash,
			"trace": errors.Details(err),
		}).Error(err.Error())
		return it, err
	}
	var resp *http.Response
	url := fmt.Sprintf("%s/actors/%s/outbox", r.BaseURL, it.SubmittedBy.Hash)
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
		f := app.LoadItemsFilter{
			Key: app.Hashes{app.Hash(hash)},
		}
		return r.LoadItem(f)
	case http.StatusGone:
		newLoc := resp.Header.Get("Location")
		hash := path.Base(newLoc)
		f := app.LoadItemsFilter{
			Key: app.Hashes{app.Hash(hash)},
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

func (r *repository) Get(u string) (*http.Response, error) {
	return r.client.Get(u)
}

func (r *repository) LoadAccounts(f app.LoadAccountsFilter) (app.AccountCollection, uint, error) {
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
		err := errors.New("unable to load from the API")
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	defer resp.Body.Close()
	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
	}
	col := ap.CollectionNew(as.ObjectID(url))
	if err = j.Unmarshal(body, &col); err != nil {
		r.logger.Error(err.Error())
		return nil, 0, err
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
	return accounts, col.TotalItems, nil
}

func (r *repository) LoadAccount(f app.LoadAccountsFilter) (app.Account, error) {
	var accounts app.AccountCollection
	var err error
	if accounts, _, err = r.LoadAccounts(f); err != nil {
		return app.Account{}, err
	}
	var ac *app.Account
	if ac, err = accounts.First(); err != nil {
		var id string
		if len(f.Key) > 0 {
			id = f.Key[0].String()
		}
		if len(f.Handle) > 0 {
			id = f.Handle[0]
		}
		return app.Account{}, errors.NotFoundf("account %q", id)
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

func loadURL(r *repository, url string) ([]byte, error) {
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
	return body, nil
}

func (r *repository) LoadInfo() (app.Info, error) {
	inf := app.Info{}
	var err error
	var body []byte

	url := fmt.Sprintf("%s/self", r.BaseURL)
	if body, err = loadURL(r, url); err != nil {
		r.logger.Error(err.Error())
		return inf, err
	}
	s := as.Service{}
	if err = j.Unmarshal(body, &s); err != nil {
		r.logger.Error(err.Error())
		return inf, err
	}
	// supplement information with what we have in /api/nodeinfo
	url = fmt.Sprintf("%s/nodeinfo", r.BaseURL)
	if body, err = loadURL(r, url); err != nil {
		r.logger.Error(err.Error())
		return inf, err
	}
	ni := nodeinfo.NodeInfo{}
	if err = j.Unmarshal(body, &ni); err != nil {
		r.logger.Error(err.Error())
		return inf, err
	}

	inf.Title = ni.Software.Name
	inf.Summary = s.Summary.First()
	inf.Description = ni.Metadata.NodeDescription
	inf.Languages = []string{"en"}
	inf.Version = ni.Software.Version
	inf.Email = fmt.Sprintf("%s@%s", "system", app.Instance.HostName)

	return inf, nil
}
