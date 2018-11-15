package frontend

import (
	"bytes"
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/juju/errors"
)

type newModel struct {
	Title         string
	InvertedTheme bool
	Content       app.Item
}

func detectMimeType(data string) string {
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
			en := strings.IndexAny(byt[st:], " \t\r\n,.:;!?")
			if en == -1 {
				en = len(byt) - st
			}
			t.Name = byt[st:st+en]
			if t.Name[0] == '#' {
				t.URL = fmt.Sprintf("%s/tags/%s", app.Instance.BaseURL, t.Name[1:])
				tags = append(tags, t)
			}
			if t.Name[0] == '@' || t.Name[0] == '~' {
				t.URL = fmt.Sprintf("%s/~%s", app.Instance.BaseURL, t.Name[1:])
				mentions = append(mentions, t)
			}

			i = i+st+en
		}
	}

	return tags, mentions
}

func ContentFromRequest(r *http.Request) (app.Item, error) {
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

	acc, _ := app.ContextCurrentAccount(r.Context())
	i.SubmittedBy = acc
	i.MimeType = detectMimeType(i.Data)
	i.Metadata = &app.ItemMetadata{}
	i.Metadata.Tags, i.Metadata.Mentions = loadTags(i.Data)
	if !i.IsLink() {
		i.MimeType = r.PostFormValue("mime-type")
	}
	if len(i.Data) > 0 {
		now := time.Now()
		i.SubmittedAt = now
		i.UpdatedAt = now
	}
	parent := r.PostFormValue("parent")
	i.Parent = &app.Item{Hash: app.Hash(parent)}
	return i, nil
}

// ShowSubmit serves GET /submit request
func ShowSubmit(w http.ResponseWriter, r *http.Request) {
	RenderTemplate(r, w, "new", newModel{Title: "New submission", InvertedTheme: isInverted(r)})
}

// HandleSubmit handles POST /submit requests
// HandleSubmit handles POST /~handle/hash requests
// HandleSubmit handles POST /year/month/day/hash requests
func HandleSubmit(w http.ResponseWriter, r *http.Request) {
	p, err := ContentFromRequest(r)
	if err != nil {
		Logger.WithContext(log.Ctx{
			"prev": err,
		}).Error("wrong http method")
		HandleError(w, r, http.StatusMethodNotAllowed, err)
		return
	}

	acc, accOk := app.ContextCurrentAccount(r.Context())
	auth, authOk := app.ContextAuthenticated(r.Context())
	if authOk && accOk && acc.IsLogged() {
		auth.WithAccount(acc)
	}

	if repo, ok := app.ContextItemSaver(r.Context()); !ok {
		Logger.Error("could not load item repository from Context")
		return
	} else {
		p, err = repo.SaveItem(p)
		if err != nil {
			Logger.WithContext(log.Ctx{
				"prev": err,
			}).Error("unable to save item")
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
	}
	if voter, ok := app.ContextVoteSaver(r.Context()); !ok {
		Logger.Error("could not load item repository from Context")
	} else {
		v := app.Vote{
			SubmittedBy: acc,
			Item:        &p,
			Weight:      1 * app.ScoreMultiplier,
		}
		if _, err := voter.SaveVote(v); err != nil {
			Logger.WithContext(log.Ctx{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
			}).Error(err.Error())
		}
	}
	Redirect(w, r, ItemPermaLink(p), http.StatusSeeOther)
}
