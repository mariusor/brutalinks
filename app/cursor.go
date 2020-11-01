package app

import (
	pub "github.com/go-ap/activitypub"
	"sort"
	"time"
)

type Cursor struct {
	after  Hash
	before Hash
	items  RenderableList
	total  uint
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

func (r RenderableList) Sorted() []Renderable {
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
				hi := Hacker(int64(ii.Score), time.Now().Sub(ii.SubmittedAt))
				hj := Hacker(int64(ij.Score), time.Now().Sub(ij.SubmittedAt))
				return oki && okj && hi > hj
			}
		}
		return ri.Date().After(rj.Date())
	})
	return rl
}
