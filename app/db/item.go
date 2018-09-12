package db

import (
	"time"

	"github.com/mariusor/littr.go/app/models"
)

type Item struct {
	Id          int64     `db:"id,auto"`
	Key         Key       `db:"key,size(64)"`
	Title       []byte    `db:"title"`
	MimeType    string    `db:"mime_type"`
	Data        []byte    `db:"data"`
	Score       int64     `db:"score"`
	SubmittedAt time.Time `db:"submitted_at"`
	SubmittedBy int64     `db:"submitted_by"`
	UpdatedAt   time.Time `db:"updated_at"`
	Flags       FlagBits  `db:"flags"`
	Metadata    Metadata  `db:"metadata"`
	Path        []byte    `db:"path"`
	FullPath    []byte
	author      *Account
}

func (i Item) Author() *Account {
	return i.author
}

func ItemFlags(f FlagBits) models.FlagBits {
	return VoteFlags(f)
}

func (i Item) Model() models.Item {
	a := i.Author().Model()
	fp := append(i.Path, byte('.'))
	fp = append(fp, i.Key.Bytes()...)
	return models.Item{
		MimeType:    i.MimeType,
		SubmittedAt: i.SubmittedAt,
		SubmittedBy: &a,
		Metadata:    ItemMetadata(i.Metadata),
		Hash:        i.Key.String(),
		Flags:       ItemFlags(i.Flags),
		Path:        i.Path,
		Data:        string(i.Data),
		Title:       string(i.Title),
		Score:       i.Score,
		UpdatedAt:   i.UpdatedAt,
		Parent:      nil,
		OP:          nil,
		FullPath:    fp,
		IsTop:       len(i.Path) == 0,
	}
}
