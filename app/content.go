package app

import (
	"html/template"
	"sort"
	"time"
	"unicode"

	pub "github.com/go-ap/activitypub"
	mark "gitlab.com/golang-commonmark/markdown"
)

type FlagBits uint8

const (
	FlagsDeleted = FlagBits(1 << iota)
	FlagsPrivate

	FlagsNone = FlagBits(0)
)

const (
	MimeTypeURL      = "application/url"
	MimeTypeHTML     = "text/html"
	MimeTypeMarkdown = "text/markdown"
	MimeTypeText     = "text/plain"
	MimeTypeSVG      = "image/svg+xml"
)

func (f *FlagBits) FromInt64() error {
	return nil
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
	AP() pub.Item
	IsValid() bool
	Type() RenderType
	Date() time.Time
}

type HasContent interface {
	Content() map[string][]byte
	Tags() TagCollection
	Mentions() TagCollection
}

// Item
type Item struct {
	Hash        Hash              `json:"hash"`
	Title       string            `json:"-"`
	MimeType    string            `json:"-"`
	Data        string            `json:"-"`
	Votes       VoteCollection    `json:"-"`
	SubmittedAt time.Time         `json:"-"`
	SubmittedBy *Account          `json:"by,omitempty"`
	UpdatedAt   time.Time         `json:"-"`
	UpdatedBy   *Account          `json:"-"`
	Flags       FlagBits          `json:"-"`
	Metadata    *ItemMetadata     `json:"-"`
	pub         pub.Item          `json:"-"`
	Parent      *Item             `json:"-"`
	OP          *Item             `json:"-"`
	Level       uint8             `json:"-"`
	children    ItemPtrCollection `json:"-"`
}

func (i *Item) ID() Hash {
	if i == nil {
		return AnonymousHash
	}
	return i.Hash
}

func (i *Item) Children() ItemPtrCollection {
	if i != nil {
		return i.children
	}
	return nil
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
	if i == nil || i.pub == nil {
		return false
	}
	isTop := false
	pub.OnObject(i.pub, func(o *pub.Object) error {
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
		if hh.Hash == s {
			return true
		}
	}
	return false
}

func (h ItemPtrCollection) Sorted() ItemPtrCollection {
	sort.SliceStable(h, func(i, j int) bool {
		ii := h[i]
		ij := h[j]
		return ii.Votes.Score()> ij.Votes.Score() || (ii.Votes.Score() == ij.Votes.Score() && ii.SubmittedAt.After(ij.SubmittedAt))
	})
	return h
}

func parentByPub (t ItemPtrCollection, cur *Item) *Item {
	var inReplyTo pub.ItemCollection
	pub.OnObject(cur.pub, func(ob *pub.Object) error {
		if ob.InReplyTo != nil {
			pub.OnCollectionIntf(ob.InReplyTo, func(col pub.CollectionInterface) error {
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
			if n.pub == nil {
				continue
			}
			if pp.GetLink().Equals(n.pub.GetLink(), false) {
				return n
			}
		}
	}
	return nil
}

func parentByHash (t ItemPtrCollection, cur *Item) *Item {
	for _, n := range t {
		if cur.Parent.IsValid() {
			if cur.Parent.Hash == n.Hash {
				return n
			}
		}
	}
	return nil
}

func reparentComments(allComments *ItemPtrCollection) {
	if len(*allComments) == 0 {
		return
	}

	var parFn = parentByHash

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
