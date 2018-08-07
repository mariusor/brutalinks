package app

import (
	"bytes"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/models"
	log "github.com/sirupsen/logrus"
	"net/http"
	"net/url"
	"time"
)

type newModel struct {
	Title         string
	InvertedTheme bool
	Content       Item
}

func detectMimeType(data []byte) string {
	u, err := url.ParseRequestURI(string(data))
	if err == nil && u != nil && !bytes.ContainsRune(data, '\n') {
		return models.MimeTypeURL
	}
	return "text/plain"
}

func ContentFromRequest(r *http.Request) (*models.Content, error) {
	if r.Method != http.MethodPost {
		return nil, errors.Errorf("invalid http method type")
	}

	i := models.Content{}
	tit := r.PostFormValue("title")
	if len(tit) > 0 {
		i.Title = []byte(tit)
	}
	dat := r.PostFormValue("data")
	if len(dat) > 0 {
		i.Data = []byte(dat)
	}
	i.SubmittedBy = CurrentAccount.Id
	i.MimeType = detectMimeType(i.Data)
	if !i.IsLink() {
		i.MimeType = r.PostFormValue("mime-type")
	}
	if len(i.Data) > 0 {
		now := time.Now()
		i.SubmittedAt = now
		i.UpdatedAt = now

		i.Key = i.GetKey()
	}

	ins := `insert into "content_items" ("key", "title", "data", "mime_type", "submitted_by", "submitted_at", "updated_at") 
		values($1, $2, $3, $4, $5, $6, $7)`

	var params = make([]interface{}, 0)
	params = append(params, i.Key)
	params = append(params, i.Title)
	params = append(params, i.Data)
	params = append(params, i.MimeType)
	params = append(params, i.SubmittedBy)
	params = append(params, i.SubmittedAt)
	params = append(params, i.UpdatedAt)

	parent := r.PostFormValue("parent")
	if len(parent) > 0 {
		ins = `insert into "content_items" ("key", "title", "data", "mime_type", "submitted_by", "submitted_at", "updated_at", "path") 
		values(
			$1, $2, $3, $4, $5, $6, $7, (select (case when "path" is not null then concat("path", '.', "key") else "key" end) 
				as "parent_path" from "content_items" where key ~* $8)::ltree
		)`
		params = append(params, parent)
	}

	res, err := Db.Exec(ins, params...)
	if err != nil {
		return nil, err
	} else {
		if rows, _ := res.RowsAffected(); rows == 0 {
			return nil, errors.Errorf("could not save item %q", i.Hash())
		}
	}

	return &i, nil
}

// ShowSubmit serves GET /submit request
func ShowSubmit(w http.ResponseWriter, r *http.Request) {
	m := newModel{Title: "New submission", InvertedTheme: IsInverted(r)}
	err := SessionStore.Save(r, w, GetSession(r))
	if err != nil {
		log.Print(err)
	}

	RenderTemplate(r, w, "new", m)
}

// HandleSubmit handles POST /submit requests
// HandleSubmit handles POST /~handle/hash requests
// HandleSubmit handles POST /year/month/day/hash requests
func HandleSubmit(w http.ResponseWriter, r *http.Request) {
	var userId = CurrentAccount.Id

	p, err := ContentFromRequest(r)
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	*p, err = models.LoadItemByHash(Db, p.Hash64())
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	AddVote(*p, 1, userId)
	i := LoadItem(*p)
	Redirect(w, r, i.PermaLink(), http.StatusSeeOther)
}
