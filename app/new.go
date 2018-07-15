package app

import (
	"bytes"
	"fmt"
		"net/http"
	"net/url"
	"time"

	"github.com/mariusor/littr.go/models"
	"github.com/gin-gonic/gin"
	"log"
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

func ContentFromRequest(r *http.Request, p []byte) (*models.Content, error) {
	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("invalid http method type")
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
	i.Path = p
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
	ins := `insert into "content_items" ("key", "title", "data", "mime_type", "submitted_by", "submitted_at", "updated_at", "path") values($1, $2, $3, $4, $5, $6, $7, $8)`
	{
		res, err := Db.Exec(ins, i.Key, i.Title, i.Data, i.MimeType, i.SubmittedBy, i.SubmittedAt, i.UpdatedAt, i.Path)
		if err != nil {
			return nil, err
		} else {
			if rows, _ := res.RowsAffected(); rows == 0 {
				return nil, fmt.Errorf("could not save item %q", i.Hash())
			}
		}
	}
	return &i, nil
}

// handleMain serves /{year}/{month}/{day}/{hash} request
func ShowSubmit(c *gin.Context) {
	w := c.Writer
	r := c.Request

	m := newModel{Title: "Submit new content"}
	err := SessionStore.Save(r, w, GetSession(r))
	if err != nil {
		log.Print(err)
	}

	RenderTemplate(r, w, "new.html", m)
}

func HandleSubmit(c *gin.Context) {
	w := c.Writer
	r := c.Request
	var userId = CurrentAccount.Id

	p, err := ContentFromRequest(r, nil)
	if err != nil {
		HandleError(w, r, http.StatusInternalServerError, err)
		return
	}
	AddVote(*p, 1, userId)
	i := LoadItem(*p, CurrentAccount.Handle)
	http.Redirect(w, r, i.PermaLink(), http.StatusMovedPermanently)
}
