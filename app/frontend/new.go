package frontend

import (
	"bytes"
	"fmt"
	"github.com/go-chi/chi"
	"github.com/mariusor/littr.go/app"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/juju/errors"
)

func detectMimeType(data string) app.MimeType {
	u, err := url.ParseRequestURI(data)
	if err == nil && u != nil && !bytes.ContainsRune([]byte(data), '\n') {
		return app.MimeTypeURL
	}
	return "text/plain"
}

func loadTags(data string) (app.TagCollection, app.TagCollection) {
	if !strings.ContainsAny(data, "#@~") {
		return nil, nil
	}
	tags := make(app.TagCollection, 0)
	mentions := make(app.TagCollection, 0)
	l := len(data)
	for i := 0; i < l; i++ {
		byt := data[i:]
		st := strings.IndexAny(byt, "#@~")
		if st >= l {
			break
		}
		if st != -1 {
			t := app.Tag{}
			en := strings.IndexAny(byt[st:], " \t\r\n'\"<>,.:;!?")
			if en == -1 {
				en = len(byt) - st
			}
			if en == 1 {
				continue
			}
			t.Name = byt[st : st+en]
			if t.Name[0] == '#' {
				// @todo(marius) :link_generation: make the tag links be generated from the corresponding route
				t.URL = fmt.Sprintf("/t/%s", t.Name[1:])
				tags = append(tags, t)
			}
			if t.Name[0] == '@' || t.Name[0] == '~' {
				t.URL = fmt.Sprintf("/~%s", t.Name[1:])
				mentions = append(mentions, t)
			}

			i = i + st + en
		}
	}

	return tags, mentions
}

func ContentFromRequest(r *http.Request, acc app.Account) (app.Item, error) {
	if r.Method != http.MethodPost {
		return app.Item{}, errors.Errorf("invalid http method type")
	}

	i := app.Item{}
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
	i.Metadata = &app.ItemMetadata{}
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

func (h *handler) ValidateItemAuthor(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		acc := h.account
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
			m, err := itemLoader.LoadItem(app.LoadItemsFilter{Key: app.Hashes{app.Hash(hash)}})
			if err != nil {
				h.logger.Error(err.Error())
				h.HandleError(w, r, errors.NewNotFound(err, "item"))
				return
			}
			if !sameHash(m.SubmittedBy.Hash, acc.Hash) {
				url.Path = path.Dir(url.Path)
				h.Redirect(w, r, url.RequestURI(), http.StatusTemporaryRedirect)
				return
			}
			next.ServeHTTP(w, r)
		}
	}
	return http.HandlerFunc(fn)
}
