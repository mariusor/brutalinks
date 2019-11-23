package frontend

import (
	"bytes"
	"fmt"
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

func loadTags(data string) (app.TagCollection, app.TagCollection) {
	if !strings.ContainsAny(data, "#@~") {
		return nil, nil
	}
	tags := make(app.TagCollection, 0)
	mentions := make(app.TagCollection, 0)
	r := regexp.MustCompile(`(?:\A|\s)([#~@]\w[\w@.]+)`)

	matches := r.FindAllSubmatch([]byte(data), -1)
	for _, submatches := range matches {
		match := submatches[1]
		t := app.Tag{}
		name := match
		isFed := false
		atP := bytes.LastIndex(name, []byte{'@'})
		if atP > 0 {
			isFed = true
		}

		if name[0] == '@' || name[0] == '~' {
			// mention
			t.Name = string(name)
			var host []byte
			if isFed {
				host = []byte(fmt.Sprintf("https://%s", name[atP+2:]))
				name = name[:atP+1]
			} else {
				host = []byte(app.Instance.BaseURL)
			}
			t.URL = fmt.Sprintf("%s/~%s", host, name[1:])
			mentions = append(mentions, t)
		}
		if match[0] == '#' {
			// @todo(marius) :link_generation: make the tag links be generated from the corresponding route
			t.Name = string(name)
			var host []byte
			if isFed {
				host = []byte(fmt.Sprintf("https://%s", name[atP+1:]))
				name = name[:atP]
			} else {
				host = []byte(app.Instance.BaseURL)
			}
			t.URL = fmt.Sprintf("%s/t/%s", host, name[1:])
			tags = append(tags, t)
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
			m, err := itemLoader.LoadItem(app.Filters{ LoadItemsFilter: app.LoadItemsFilter{Key: app.Hashes{app.Hash(hash)}}})
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
