package cmd

import (
	"fmt"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"github.com/mariusor/littr.go/app/log"
	"github.com/mmcdole/gofeed"
	"net/url"
	"time"
)

var sys = app.Account{
	Hash:   "dc6f5f5bf55bc1073715c98c69fa7ca8",
	Handle: "system",
}

func PoachFeed(u string, since time.Duration) error {
	var err error
	fp := gofeed.NewParser()
	doc, err := fp.ParseURL(u)
	if err != nil {
		return errors.Annotatef(err, "failed to fetch rss feed %s", u)
	}

	baseURL := ""
	feedURL, _ := url.ParseRequestURI(u)
	if doc.Link != "" {
		feedURL, err = url.ParseRequestURI(doc.Link)
	}
	if err == nil {
		baseURL = feedURL.Host
	}
	for _, l := range doc.Items {
		acct := sys
		if l.Author.Name != "" {
			acct = app.Account{}
			if baseURL != "" {
				acct.Handle = fmt.Sprintf("%s/~%s", baseURL, l.Author.Name)
			}
			if l.Author.Email != "" {
				acct.Email = l.Author.Email
			}
			_, err = db.Config.LoadAccount(app.LoadAccountsFilter{
				Handle: []string{acct.Handle},
			})
			if err != nil {
				acct.CreatedAt = time.Now()
				acct, err = db.Config.SaveAccount(acct)
				if err != nil {
					Logger.WithContext(log.Ctx{
						"key":    acct.Hash.String(),
						"handle": acct.Handle,
						"err" : err.Error(),
					}).Error("unable to save new account")
				}
			}
			if err != nil || acct.Handle == "" {
				acct = sys
			}
		}
		item := app.Item{
			Data:        l.Link,
			SubmittedAt: *l.PublishedParsed,
			SubmittedBy: &acct,
			Title:       l.Title,
			MimeType:    app.MimeTypeURL,
		}
		item, err = db.Config.SaveItem(item)
		if err != nil {
			Logger.WithContext(log.Ctx{
				"title": item.Title,
				"data":  item.Data,
				"err" : err.Error(),
			}).Errorf("unable to save new item")
		}
	}
	return nil
}
