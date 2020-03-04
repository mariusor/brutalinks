package app

import (
	"fmt"
	pub "github.com/go-ap/activitypub"
	"html/template"
	"net/url"
	"strings"
	"time"

	mark "gitlab.com/golang-commonmark/markdown"
)

type FlagBits uint8

const (
	FlagsDeleted = FlagBits(1 << iota)
	FlagsPrivate

	FlagsNone = FlagBits(0)
)

const MimeTypeURL = MimeType("application/url")
const MimeTypeHTML = MimeType("text/html")
const MimeTypeMarkdown = MimeType("text/markdown")
const MimeTypeText = MimeType("text/plain")
const RandomSeedSelectedByDiceRoll = 777

func (f *FlagBits) FromInt64() error {
	return nil
}

type ItemCollection []Item

func Markdown(data string) template.HTML {
	md := mark.New(
		mark.HTML(true),
		mark.Tables(true),
		mark.Linkify(false),
		mark.Breaks(false),
		mark.Typographer(true),
		mark.XHTMLOutput(false),
	)

	h := md.RenderToString([]byte(data))
	return template.HTML(h)
}

// HasMetadata
func (i *Item) HasMetadata() bool {
	return i != nil && i.Metadata != nil
}

// IsFederated
func (i Item) IsFederated() bool {
	return !i.IsLocal()
}

// IsLocal
func (i Item) IsLocal() bool {
	if !i.HasMetadata() {
		return true
	}
	if len(i.Metadata.ID) > 0 {
		return HostIsLocal(i.Metadata.ID)
	}
	if len(i.Metadata.URL) > 0 {
		return HostIsLocal(i.Metadata.URL)
	}
	return true
}

const Edit = "edit"
const Delete = "rm"
const Report = "bad"
const Yay = "yay"
const Nay = "nay"

type RenderType int

const (
	Comment = iota
	Follow
	Appreciation
)

type Renderable interface {
	Type() RenderType
}

func (f FollowRequest) Type() RenderType {
	return Follow
}

type Item struct {
	Hash        Hash           `json:"hash"`
	Title       string         `json:"-"`
	MimeType    MimeType       `json:"-"`
	Data        string         `json:"-"`
	Score       int            `json:"-"`
	SubmittedAt time.Time      `json:"-"`
	SubmittedBy *Account       `json:"-"`
	UpdatedAt   time.Time      `json:"-"`
	UpdatedBy   *Account       `json:"-"`
	Flags       FlagBits       `json:"-"`
	Metadata    *ItemMetadata  `json:"-"`
	pub         *pub.Object    `json:"-"`
	IsTop       bool           `json:"-"`
	Parent      *Item          `json:"-"`
	OP          *Item          `json:"-"`
	Voted       uint8          `json:"-"`
	Level       uint8          `json:"-"`
	Edit        bool           `json:"-"`
	Children    ItemCollection `json:"-"`
}

func (i Item) Type() RenderType {
	return Comment
}

func (i ItemCollection) Contains(cc Item) bool {
	for _, com := range i {
		if HashesEqual(com.Hash, cc.Hash) {
			return true
		}
	}
	return false
}

func (i ItemCollection) getItemsHashes() Hashes {
	var items = make(Hashes, len(i))
	for k, com := range i {
		items[k] = com.Hash
	}
	return items
}

func mimeTypeTagReplace(m MimeType, t Tag) string {
	var cls string

	if t.Type == TagTag {
		cls = "tag"
	}
	if t.Type == TagMention {
		cls = "mention"
	}

	return fmt.Sprintf("<a href='%s' class='%s'>%s</a>", t.URL, cls, t.Name)
}

func inRange(n string, nn map[string]string) bool {
	for k := range nn {
		if k == n {
			return true
		}
	}
	return false
}

func replaceTagsInItem(cur Item) string {
	dat := cur.Data
	if cur.Metadata == nil {
		return dat
	}
	replaces := make(map[string]string, 0)
	if cur.Metadata.Tags != nil {
		for _, t := range cur.Metadata.Tags {
			name := fmt.Sprintf("#%s", t.Name)
			if inRange(name, replaces) {
				continue
			}
			replaces[name] = mimeTypeTagReplace(cur.MimeType, t)
		}
	}
	if cur.Metadata.Mentions != nil {
		for idx, t := range cur.Metadata.Mentions {
			lbl := fmt.Sprintf(":::MENTION_%d:::", idx)
			if inRange(lbl, replaces) {
				continue
			}
			if u, err := url.Parse(t.URL); err == nil && len(u.Host) > 0 {
				nameAtT := fmt.Sprintf("~%s@%s", t.Name, u.Host)
				nameAtA := fmt.Sprintf("@%s@%s", t.Name, u.Host)
				dat = strings.ReplaceAll(dat, nameAtT, lbl)
				dat = strings.ReplaceAll(dat, nameAtA, lbl)
			}
			nameT := fmt.Sprintf("~%s", t.Name)
			nameA := fmt.Sprintf("@%s", t.Name)
			dat = strings.ReplaceAll(dat, nameT, lbl)
			dat = strings.ReplaceAll(dat, nameA, lbl)
			replaces[lbl] = mimeTypeTagReplace(cur.MimeType, t)
		}
	}

	for to, repl := range replaces {
		dat = strings.ReplaceAll(dat, to, repl)
	}
	return dat
}

func removeCurElementParentComments(com *ItemCollection) {
	first := (*com)[0]
	lvl := first.Level
	keepComments := make(ItemCollection, 0)
	for _, cur := range *com {
		if cur.Level >= lvl {
			keepComments = append(keepComments, cur)
		}
	}
	*com = keepComments
}

func addLevelComments(allComments ItemCollection) {
	leveled := make(Hashes, 0)
	var setLevel func(ItemCollection)

	setLevel = func(com ItemCollection) {
		for _, cur := range com {
			if leveled.Contains(cur.Hash) {
				break
			}
			leveled = append(leveled, cur.Hash)
			if len(cur.Children) > 0 {
				for _, child := range cur.Children {
					child.Level = cur.Level + 1
					setLevel(cur.Children)
				}
			}
		}
	}
	setLevel(allComments)
}

func reparentComments(allComments ItemCollection) {
	parFn := func(t ItemCollection, cur Item) *Item {
		for _, n := range t {
			if cur.Parent.IsValid() {
				if HashesEqual(cur.Parent.Hash, n.Hash) {
					return &n
				}
			}
		}
		return nil
	}

	first := allComments[0]
	for _, cur := range allComments {
		if par := parFn(allComments, cur); par != nil {
			if HashesEqual(first.Hash, cur.Hash) {
				continue
			}
			cur.Parent = par
			par.Children = append(par.Children, cur)
		}
	}
}
