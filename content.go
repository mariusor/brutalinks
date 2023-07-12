package brutalinks

import (
	"html/template"
	"sort"
	"strings"
	"time"
	"unicode"

	vocab "github.com/go-ap/activitypub"
	mark "gitlab.com/golang-commonmark/markdown"
)

type FlagBits uint8

const (
	FlagsDeleted = FlagBits(1 << iota)
	FlagsOperator
	FlagsModerator
	FlagsApplication
	FlagsGroup
	FlagsService
	FlagsPrivate

	FlagsNone = FlagBits(0)
)

const (
	MimeTypeURL      = "application/url"
	MimeTypeHTML     = "text/html"
	MimeTypeMarkdown = "text/markdown"
	MimeTypeText     = "text/plain"
	MimeTypeSVG      = "image/svg+xml"
	MimeTypeCss      = "text/css"
)

func (f *FlagBits) FromInt64() error {
	return nil
}

func (f FlagBits) MarshalJSON() ([]byte, error) {
	pieces := make([]string, 0)
	if f|FlagsDeleted == f {
		pieces = append(pieces, "Deleted")
	}
	if f|FlagsOperator == f {
		pieces = append(pieces, "Operator")
	}
	if f|FlagsModerator == f {
		pieces = append(pieces, "Moderator")
	}
	if f|FlagsApplication == f {
		pieces = append(pieces, "Application")
	}
	if f|FlagsGroup == f {
		pieces = append(pieces, "Group")
	}
	if f|FlagsService == f {
		pieces = append(pieces, "Service")
	}
	if f|FlagsPrivate == f {
		pieces = append(pieces, "Private")
	}
	if len(pieces) == 0 {
		return []byte("None"), nil
	}
	return []byte(`"` + strings.Join(pieces, "|") + `"`), nil
}

type ItemCollection []Item

var MdPolicy = mark.New(
	mark.HTML(true),
	mark.Tables(true),
	mark.Linkify(false),
	mark.Breaks(false),
	mark.Typographer(false),
	mark.XHTMLOutput(false),
)

// Markdown outputs the markdown render of a string
func Markdown(data string) template.HTML {
	return template.HTML(MdPolicy.RenderToString([]byte(data)))
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
	CommentType RenderType = iota
	FollowType
	AppreciationType
	ActorType
	ModerationType
)

type Renderable interface {
	ID() Hash
	AP() vocab.Item
	IsValid() bool
	Type() RenderType
	Date() time.Time
	Children() *RenderableList
}

type HasContent interface {
	Content() map[string][]byte
	Tags() TagCollection
	Mentions() TagCollection
}

// Item
type Item struct {
	Hash        Hash           `json:"hash"`
	Title       string         `json:"-"`
	MimeType    string         `json:"-"`
	Data        string         `json:"-"`
	Votes       VoteCollection `json:"-"`
	SubmittedAt time.Time      `json:"-"`
	SubmittedBy *Account       `json:"by,omitempty"`
	UpdatedAt   time.Time      `json:"-"`
	UpdatedBy   *Account       `json:"-"`
	Flags       FlagBits       `json:"-"`
	Metadata    *ItemMetadata  `json:"-"`
	Pub         vocab.Item     `json:"-"`
	Parent      Renderable     `json:"-"`
	OP          Renderable     `json:"-"`
	Level       uint8          `json:"-"`
	children    RenderableList `json:"-"`
}

func (i *Item) ID() Hash {
	if i == nil {
		return AnonymousHash
	}
	return i.Hash
}

func (i *Item) Children() *RenderableList {
	return &i.children
}

func (i *Item) Type() RenderType {
	return CommentType
}

func (i Item) Date() time.Time {
	return i.SubmittedAt
}

func (i Item) Score() int {
	return i.Votes.Score()
}

// IsTop returns true if current item is a top level submission
func (i *Item) IsTop() bool {
	if i == nil || i.Pub == nil {
		return false
	}
	isTop := false
	vocab.OnObject(i.Pub, func(o *vocab.Object) error {
		if o.InReplyTo == nil {
			isTop = true
		}
		return nil
	})
	return isTop
}

func (i ItemCollection) Contains(cc Item) bool {
	for _, com := range i {
		if com.Hash == cc.Hash {
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

func inRange(n string, nn map[string]string) bool {
	for k := range nn {
		if k == n {
			return true
		}
	}
	return false
}

func replaceBetweenPos(d, r []byte, st, end int) []byte {
	if st < 0 || end > len(d) {
		return d
	}
	if end < len(d) {
		r = append(r, d[end:]...)
	}
	return append(d[:st], r...)
}

func isWordDelimiter(b byte) bool {
	return unicode.Is(unicode.Number, rune(b)) ||
		unicode.Is(unicode.Letter, rune(b)) ||
		unicode.Is(unicode.Punct, rune(b))
}

func addLevels(allComments RenderableList) RenderableList {
	if len(allComments) == 0 {
		return nil
	}

	leveled := make(Hashes, 0)
	var setLevel func(RenderableList)

	setLevel = func(com RenderableList) {
		for _, cur := range com {
			if cur == nil || leveled.Contains(cur.ID()) {
				break
			}
			leveled = append(leveled, cur.ID())
			if cur.Children() == nil {
				break
			}
			for _, child := range *cur.Children() {
				if c, ok := child.(*Item); ok {
					c.Level = c.Level + 1
					setLevel(c.children)
				}
			}
		}
	}
	setLevel(allComments)
	return allComments
}

type ItemPtrCollection []*Item

func (h ItemPtrCollection) Contains(it Item) bool {
	for _, hh := range h {
		if hh.Hash == it.Hash {
			return true
		}
	}
	return false
}

func (h ItemPtrCollection) Sorted() ItemPtrCollection {
	sort.SliceStable(h, func(i, j int) bool {
		ii := h[i]
		ij := h[j]
		return ii.Votes.Score() > ij.Votes.Score() || (ii.Votes.Score() == ij.Votes.Score() && ii.SubmittedAt.After(ij.SubmittedAt))
	})
	return h
}

func parentByPub(t ItemPtrCollection, cur *Item) *Item {
	var inReplyTo vocab.ItemCollection
	vocab.OnObject(cur.Pub, func(ob *vocab.Object) error {
		if ob.InReplyTo != nil {
			vocab.OnCollectionIntf(ob.InReplyTo, func(col vocab.CollectionInterface) error {
				inReplyTo = col.Collection()
				return nil
			})
		}
		return nil
	})
	if len(inReplyTo) == 0 {
		return nil
	}
	for _, n := range t {
		for _, pp := range inReplyTo {
			if n.Pub == nil {
				continue
			}
			if pp.GetLink().Equals(n.Pub.GetLink(), false) {
				return n
			}
		}
	}
	return nil
}

func parentByHash(t RenderableList, cur Renderable) Renderable {
	for _, n := range t {
		switch c := cur.(type) {
		case *Item:
			if c.Parent != nil && c.Parent.IsValid() {
				if c.Parent.ID() == n.ID() {
					return n
				}
			}
		case *Account:
			if c.CreatedBy.IsValid() {
				if c.CreatedBy.ID() == n.ID() {
					return n
				}
			}
		}
	}
	return nil
}

func reparentRenderables(allComments RenderableList) RenderableList {
	if len(allComments) == 0 {
		return allComments
	}

	var parFn = parentByHash

	retComments := make(RenderableList, 0)
	for _, cur := range allComments {
		if par := parFn(allComments, cur); par != nil {
			if par.Children().Contains(cur) {
				continue
			}
			par.Children().Append(cur)
			switch c := cur.(type) {
			case *Item:
				c.Parent, _ = par.(*Item)
			case *Account:
				c.Parent, _ = par.(*Account)
			}
		} else {
			if cur == nil || retComments.Contains(cur) {
				continue
			}
			retComments = append(retComments, cur)
		}
	}
	return addLevels(retComments)
}
