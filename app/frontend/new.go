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

func detectMimeType(data []byte) string {
	u, err := url.ParseRequestURI(string(data))
	if err == nil && u != nil && !bytes.ContainsRune(data, '\n') {
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
	i.SubmittedBy = CurrentAccount
	i.MimeType = i.Data
	if !i.IsLink() {
		i.MimeType = r.PostFormValue("mime-type")
	}
	if len(i.Data) > 0 {
		now := time.Now()
		i.SubmittedAt = now
		i.UpdatedAt = now
	}
	parent := r.PostFormValue("parent")
	i.Parent = &models.Item{Hash: parent}
	return i, nil
}

// ShowSubmit serves GET /submit request
func ShowSubmit(w http.ResponseWriter, r *http.Request) {
	m := newModel{Title: "New submission", InvertedTheme: isInverted(r)}
	err := SessionStore.Save(r, w, GetSession(r))
	if err != nil {
		log.WithFields(log.Fields{}).Error(err)
	}

	RenderTemplate(r, w, "new", m)
}

// HandleSubmit handles POST /submit requests
// HandleSubmit handles POST /~handle/hash requests
// HandleSubmit handles POST /year/month/day/hash requests
func HandleSubmit(w http.ResponseWriter, r *http.Request) {
	p, err := ContentFromRequest(r)
	if err != nil {
		log.WithFields(log.Fields{}).Errorf("wrong http method: %s", err)
	}
	p, err = models.SaveItem(p)
	if err != nil {
		log.WithFields(log.Fields{}).Errorf("unable to save item: %s", err)
		HandleError(w, r, http.StatusInternalServerError, err)
	}
	//AddVote(p, 1, p.AttributedTo.Hash)
	Redirect(w, r, permaLink(p), http.StatusSeeOther)
}
