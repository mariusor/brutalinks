package app

import (
	"fmt"
	pub "github.com/go-ap/activitypub"
	"html/template"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	mark "gitlab.com/golang-commonmark/markdown"
)

type FlagBits uint8

const (
	FlagsDeleted = FlagBits(1 << iota)
	FlagsPrivate

	FlagsNone = FlagBits(0)
)

const MimeTypeURL = "application/url"
const MimeTypeHTML = "text/html"
const MimeTypeMarkdown = "text/markdown"
const MimeTypeText = "text/plain"

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
	Comment RenderType = iota
	Follow
	AppreciationLike
	AppreciationDislike
	Actor
	ModerationBlock
	ModerationIgnored
	ModerationReported
)

type Renderable interface {
	AP() pub.Item
	IsValid() bool
	Type() RenderType
	Date() time.Time
}

type RenderableList []Renderable

func (r RenderableList) Items() ItemCollection {
	items := make(ItemCollection, 0)
	for _, ren := range r {
		if it, ok := ren.(*Item); ok {
			items = append(items, *it)
		}
	}
	return items
}

func (r RenderableList) Follows() FollowRequests {
	follows := make(FollowRequests, 0)
	for _, ren := range r {
		if it, ok := ren.(*FollowRequest); ok {
			follows = append(follows, *it)
		}
	}
	return follows
}

// Item
type Item struct {
	Hash        Hash              `json:"hash"`
	Title       string            `json:"-"`
	MimeType    string            `json:"-"`
	Data        string            `json:"-"`
	Score       int               `json:"-"`
	SubmittedAt time.Time         `json:"-"`
	SubmittedBy *Account          `json:"by,omitempty"`
	UpdatedAt   time.Time         `json:"-"`
	UpdatedBy   *Account          `json:"-"`
	Flags       FlagBits          `json:"-"`
	Metadata    *ItemMetadata     `json:"-"`
	pub         pub.Item          `json:"-"`
	IsTop       bool              `json:"-"`
	Parent      *Item             `json:"-"`
	OP          *Item             `json:"-"`
	Voted       uint8             `json:"-"`
	Level       uint8             `json:"-"`
	Edit        bool              `json:"-"`
	children    ItemPtrCollection `json:"-"`
}

func (i *Item) Children() ItemPtrCollection {
	if i != nil {
		return i.children
 	} 
 	return nil
} 

func (i *Item) Type() RenderType {
	return Comment
}

func (i Item) Date() time.Time {
	return i.SubmittedAt
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

func mimeTypeTagReplace(m string, t Tag) string {
	var cls string

	if t.Type == TagTag {
		cls = "tag"
	}
	if t.Type == TagMention {
		cls = "mention"
	}

	return fmt.Sprintf("<a href='%s' rel='%s'>%s</a>", t.URL, cls, t.Name)
}

func inRange(n string, nn map[string]string) bool {
	for k := range nn {
		if k == n {
			return true
		}
	}
	return false
}

func replaceBetweenPos(d, r string, st, end int) string {
	if st < 0 || end > len(d) {
		return d
	}
	if end < len(d) {
		r += d[end:]
	}
	return d[:st] + r
}

func isWordDelimiter(b byte) bool {
	return unicode.Is(unicode.Number, rune(b)) ||
		unicode.Is(unicode.Letter, rune(b)) ||
		unicode.Is(unicode.Punct, rune(b))
}

func replaceTag(d *string, t Tag, w string) {
	inWord := func(d string, i, end int) bool {
		dl := len(d)
		if i < 1 || end > dl {
			return false
		}
		before := isWordDelimiter(d[i-1])
		after := true
		if end < dl {
			after = isWordDelimiter(d[end])
		}
		return before && after
	}

	var base []string
	base = append(base, t.Name)
	if u, err := url.Parse(t.URL); err == nil && len(u.Host) > 0 {
		base = append(base, t.Name+`@`+u.Host)
	}

	var pref [][]byte
	if t.Type == TagMention {
		pref = [][]byte{{'~'}, {'@'}}
	} else {
		pref = [][]byte{{'#'}}
	}
	var search [][]byte
	for _, p := range pref {
		for _, b := range base {
			s := append(p, b...)
			search = append(search, s)
		}
	}
	for _, s := range search {
		end := 0
		for {
			if end >= len(*d) {
				break
			}
			inx := strings.Index((*d)[end:], string(s))
			if inx < 0 {
				break
			}
			pos := end + inx
			end = pos + len(s)
			if end > len(*d) {
				break
			}
			if !inWord(*d, pos, end) {
				*d = replaceBetweenPos(*d, w, pos, end)
			}
		}
	}
}

func replaceTagsInItem(cur Item) string {
	if !cur.HasMetadata() {
		return cur.Data
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
			replaceTag(&cur.Data, t, lbl)
			replaces[lbl] = mimeTypeTagReplace(cur.MimeType, t)
		}
	}

	for to, repl := range replaces {
		cur.Data = strings.ReplaceAll(cur.Data, to, repl)
	}
	return cur.Data
}

func (c TagCollection) Contains(t Tag) bool {
	for _, tt := range c {
		if tt.Type == t.Type && tt.Name == t.Name && tt.URL == t.URL {
			return true
		}
	}
	return false
}

func loadTags(data string) (TagCollection, TagCollection) {
	if !strings.ContainsAny(data, "#@~") {
		return nil, nil
	}
	tags := make(TagCollection, 0)
	mentions := make(TagCollection, 0)

	r := regexp.MustCompile(`(?:\A|\s)((?:[~@]\w+)(?:@\w+.\w+)?|(?:#[\w-]{3,}))`)
	matches := r.FindAllSubmatch([]byte(data), -1)

	for _, sub := range matches {
		t := getTagFromBytes(sub[1])
		if t.Type == TagMention && !mentions.Contains(t) {
			mentions = append(mentions, t)
		}
		if t.Type == TagTag && !tags.Contains(t) {
			tags = append(tags, t)
		}
	}
	return tags, mentions
}

func addLevelComments(allComments []*Item) {
	if len(allComments) == 0 {
		return
	}

	leveled := make(Hashes, 0)
	var setLevel func([]*Item)

	setLevel = func(com []*Item) {
		for _, cur := range com {
			if cur == nil || leveled.Contains(cur.Hash) {
				break
			}
			leveled = append(leveled, cur.Hash)
			if len(cur.children) > 0 {
				for _, child := range cur.children {
					child.Level = cur.Level + 1
					setLevel(cur.children)
				}
			}
		}
	}
	setLevel(allComments)
}

type ItemPtrCollection []*Item

func (h ItemPtrCollection) Contains(s Hash) bool {
	for _, hh := range h {
		if HashesEqual(hh.Hash, s) {
			return true
		}
	}
	return false
}

func reparentComments(allComments *ItemPtrCollection) {
	if len(*allComments) == 0 {
		return
	}

	parFn := func(t ItemPtrCollection, cur *Item) *Item {
		for _, n := range t {
			if cur.Parent.IsValid() {
				if HashesEqual(cur.Parent.Hash, n.Hash) {
					return n
				}
			}
		}
		return nil
	}

	retComments := make(ItemPtrCollection, 0)
	for _, cur := range *allComments {
		if par := parFn(*allComments, cur); par != nil {
			if par.children.Contains(cur.Hash) {
				continue
			}
			par.children = append(par.children, cur)
			cur.Parent = par
		} else {
			if cur == nil || retComments.Contains(cur.Hash) {
				continue
			}
			retComments = append(retComments, cur)
		}
	}
	*allComments = retComments
}
