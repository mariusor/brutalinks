package cmd

import (
	"fmt"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/db"
	"time"
)

func UpdateScores(key string, handle string, since time.Duration, items bool, accounts bool) error {
	var err error
	// recount all votes for content items
	var scores []app.Score
	if accounts {
		which := ""
		val := ""
		if handle != "" || key != "" {
			if len(handle) > 0 {
				which = "handle"
				val = handle
			} else {
				which = "key"
				val = key
			}
		}
		scores, err = db.LoadScoresForAccounts(since, which, val)
	} else if items {
		scores, err = db.LoadScoresForItems(since, key)
	}
	if err != nil {
		return err
	}

	sql := `update "%s" set score = $1 where id = $2;`
	for _, score := range scores {
		var col string
		if score.Type == app.ScoreItem {
			col = `content_items`
		} else {
			col = `accounts`
		}
		_, err := db.Config.DB.Exec(fmt.Sprintf(sql, col), score.Score, score.ID)
		if err != nil {
			return err
		}
	}
	return nil
}
