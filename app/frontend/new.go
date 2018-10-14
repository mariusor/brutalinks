package frontend

import (
	"bytes"
	"net/http"
	"net/url"
	"time"

	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app/models"
	log "github.com/sirupsen/logrus"
)

type newModel struct {
	Title         string
	InvertedTheme bool
	Content       models.Item
}

func detectMimeType(data string) string {
	u, err := url.ParseRequestURI(data)
	if err == nil && u != nil && !bytes.ContainsRune([]byte(data), '\n') {
		return models.MimeTypeURL
	}
	return "text/plain"
}

func ContentFromRequest(r *http.Request) (models.Item, error) {
	if r.Method != http.MethodPost {
		return models.Item{}, errors.Errorf("invalid http method type")
	}

	i := models.Item{}
	tit := r.PostFormValue("title")
	if len(tit) > 0 {
		i.Title = tit
	}
	dat := r.PostFormValue("data")
	if len(dat) > 0 {
		i.Data = dat
	}

	acc, _ := models.ContextCurrentAccount(r.Context())
	i.SubmittedBy = acc
	i.MimeType = detectMimeType(i.Data)
	if !i.IsLink() {
		i.MimeType = r.PostFormValue("mime-type")
	}
	if len(i.Data) > 0 {
		now := time.Now()
		i.SubmittedAt = now
		i.UpdatedAt = now
	}
	parent := r.PostFormValue("parent")
	i.Parent = &models.Item{Hash: models.Hash(parent)}
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
		Logger.WithFields(log.Fields{}).Errorf("wrong http method: %s", err)
		HandleError(w, r, http.StatusMethodNotAllowed, err)
		return
	}

	acc, accOk := models.ContextCurrentAccount(r.Context())
	auth, authOk := models.ContextAuthenticated(r.Context())
	if authOk && accOk && acc.IsLogged() {
		auth.WithAccount(acc)
	}

	if repo, ok := models.ContextItemSaver(r.Context()); !ok {
		Logger.WithFields(log.Fields{}).Errorf("could not load item repository from Context")
		return
	} else {
		p, err = repo.SaveItem(p)
		if err != nil {
			Logger.WithFields(log.Fields{}).Errorf("unable to save item: %s", err)
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
	}
	if voter, ok := models.ContextVoteSaver(r.Context()); !ok {
		Logger.WithFields(log.Fields{}).Errorf("could not load item repository from Context")
	} else {
		v := models.Vote{
			SubmittedBy: acc,
			Item:        &p,
			Weight:      1 * models.ScoreMultiplier,
		}
		if _, err := voter.SaveVote(v); err != nil {
			Logger.WithFields(log.Fields{
				"hash":   v.Item.Hash,
				"author": v.SubmittedBy.Handle,
				"weight": v.Weight,
			}).Error(err)
		}
	}
	Redirect(w, r, ItemPermaLink(p), http.StatusSeeOther)
}
