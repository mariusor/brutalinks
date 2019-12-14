package frontend

import (
	"bytes"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-chi/chi"
	"github.com/mariusor/littr.go/app"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/go-ap/errors"
)

func detectMimeType(data string) app.MimeType {
	u, err := url.ParseRequestURI(data)
	if err == nil && u != nil && !bytes.ContainsRune([]byte(data), '\n') {
		return app.MimeTypeURL
	}
	return "text/plain"
}

func getTagFromBytes(d []byte) app.Tag {
	var name, host []byte
	t := app.Tag{}

	if ind := bytes.LastIndex(d, []byte{'@'}); ind > 1 {
		name = d[1:ind]
		host = []byte(fmt.Sprintf("https://%s", d[ind+1:]))
	} else {
		name = d[1:]
		host = []byte(app.Instance.BaseURL)
	}
	if d[0] == '@' || d[0] == '~' {
		// mention
		t.Type = app.TagMention
		t.Name = string(name)
		t.URL = fmt.Sprintf("%s/~%s", host, name)
	}
	if d[0] == '#' {
		// @todo(marius) :link_generation: make the tag links be generated from the corresponding route
		t.Type = app.TagTag
		t.Name = string(name)
		t.URL = fmt.Sprintf("%s/t/%s", host, name)
	}
	return t
}

func loadTags(data string) (app.TagCollection, app.TagCollection) {
	if !strings.ContainsAny(data, "#@~") {
		return nil, nil
	}
	tags := make(app.TagCollection, 0)
	mentions := make(app.TagCollection, 0)

	r := regexp.MustCompile(`(?:\A|\s)((?:[~@]\w+)(?:@\w+.\w+)?|(?:#\w{4,}))`)
	matches := r.FindAllSubmatch([]byte(data), -1)

	for _, sub := range matches {
		t := getTagFromBytes(sub[1])
		if t.Type == app.TagMention {
			mentions = append(mentions, t)
		}
		if t.Type == app.TagTag {
			tags = append(tags, t)
		}
	}
	return tags, mentions
}

func ContentFromRequest(r *http.Request, acc app.Account) (app.Item, error) {
	if r.Method != http.MethodPost {
		return app.Item{}, errors.Errorf("invalid http method type")
	}

	var receiver *app.Account
	var err error
	i := app.Item{}
	i.Metadata = &app.ItemMetadata{}
	if receiver, err = accountFromRequestHandle(r); err == nil && chi.URLParam(r, "hash") == "" {
		i.MakePrivate()
		to := app.Account{}
		to.FromActivityPub(pub.IRI(receiver.Metadata.ID))
		i.Metadata.To = []*app.Account{&to,}
	}

	tit := r.PostFormValue("title")
	if len(tit) > 0 {
		i.Title = tit
	}
	dat := r.PostFormValue("data")
	if len(dat) > 0 {
		i.Data = dat
	}

	i.SubmittedBy = &acc
	i.MimeType = detectMimeType(i.Data)

	i.Metadata.Tags, i.Metadata.Mentions = loadTags(i.Data)
	if !i.IsLink() {
		i.MimeType = app.MimeType(r.PostFormValue("mime-type"))
	}
	if len(i.Data) > 0 {
		now := time.Now().UTC()
		i.SubmittedAt = now
		i.UpdatedAt = now
	}
	parent := r.PostFormValue("parent")
	if len(parent) > 0 {
		i.Parent = &app.Item{Hash: app.Hash(parent)}
	}
	op := r.PostFormValue("op")
	if len(op) > 0 {
		i.OP = &app.Item{Hash: app.Hash(op)}
	}
	hash := r.PostFormValue("hash")
	if len(hash) > 0 {
		i.Hash = app.Hash(hash)
	}
	return i, nil
}

// ShowSubmit serves GET /submit request
func (h *handler) ShowSubmit(w http.ResponseWriter, r *http.Request) {
	h.RenderTemplate(r, w, "new", contentModel{Title: "New submission"})
}

func (h *handler) ValidatePermissions(actions ...string) func(http.Handler) http.Handler {
	if len(actions) == 0 {
		return h.ValidateItemAuthor
	}
	// @todo(marius): implement permission logic
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

func (h *handler) RedirectToLogin(w http.ResponseWriter, r *http.Request, errs ...error) {
	h.Redirect(w, r, "/login", http.StatusMovedPermanently)
}

func (h *handler) ValidateLoggedIn(eh app.ErrorHandler) app.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			if !h.account(r).IsLogged() {
				e := errors.Unauthorizedf("Please login to perform this action")
				h.logger.Errorf("%s", e)
				eh(w, r, e)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

func (h *handler) ValidateItemAuthor(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		acc := h.account(r)
		hash := chi.URLParam(r, "hash")
		url := r.URL
		action := path.Base(url.Path)
		if len(hash) > 0 && action != hash {
			val := r.Context().Value(app.RepositoryCtxtKey)
			itemLoader, ok := val.(app.CanLoadItems)
			if !ok {
				h.logger.Error("could not load item repository from Context")
				return
			}
			m, err := itemLoader.LoadItem(app.Filters{LoadItemsFilter: app.LoadItemsFilter{Key: app.Hashes{app.Hash(hash)}}})
			if err != nil {
				h.logger.Error(err.Error())
				h.HandleErrors(w, r, errors.NewNotFound(err, "item"))
				return
			}
			if !app.HashesEqual(m.SubmittedBy.Hash, acc.Hash) {
				url.Path = path.Dir(url.Path)
				h.Redirect(w, r, url.RequestURI(), http.StatusTemporaryRedirect)
				return
			}
			next.ServeHTTP(w, r)
		}
	}
	return http.HandlerFunc(fn)
}
