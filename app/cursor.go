package app

import (
	"sort"
	"time"

	pub "github.com/go-ap/activitypub"
)

type Cursor struct {
	after  Hash
	before Hash
	items  RenderableList
	total  uint
}

func NewCursor() *Cursor {
	return &Cursor{items: make(RenderableList)}
}

var emptyCursor = Cursor{}

type colCursor struct {
	filters *Filters
	loaded  int
	items   pub.ItemCollection
}

type RenderableList map[Hash]Renderable

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

func (r *RenderableList) Merge(other RenderableList) {
	for k, it := range other {
		(*r)[k] = it
	}
}

func (r *RenderableList) Append(others ...Renderable) {
	for _, o := range others {
		(*r)[o.ID()] = o
	}
}

func ByDate(r RenderableList) []Renderable {
	rl := make([]Renderable, 0)
	for _, rr := range r {
		rl = append(rl, rr)
	}
	sort.SliceStable(rl, func(i, j int) bool {
		ri := rl[i]
		rj := rl[j]
		if ri.Type() == rj.Type() {
			switch ri.Type() {
			case CommentType:
				ii, oki := ri.(*Item)
				ij, okj := rj.(*Item)
				subOrder := ii.SubmittedAt.After(ij.SubmittedAt)
				subSame := ii.SubmittedAt.Sub(ij.SubmittedAt) == 0
				updOrder := ii.UpdatedAt.After(ij.UpdatedAt)
				return oki && okj && (subOrder || (subSame && updOrder))
			}
		}
		return ri.Date().After(rj.Date())
	})
	return rl
}

func ByScore(r RenderableList) []Renderable {
	rl := make([]Renderable, 0)
	for _, rr := range r {
		rl = append(rl, rr)
	}
	sort.SliceStable(rl, func(i, j int) bool {
		ri := rl[i]
		rj := rl[j]
		if ri.Type() == rj.Type() {
			switch ri.Type() {
			case CommentType:
				ii, oki := ri.(*Item)
				ij, okj := rj.(*Item)
				hi := Hacker(int64(ii.Votes.Score()), time.Now().Sub(ii.SubmittedAt))
				hj := Hacker(int64(ij.Votes.Score()), time.Now().Sub(ij.SubmittedAt))
				return oki && okj && hi > hj
			}
		}
		return ri.Date().After(rj.Date())
	})
	return rl
}
