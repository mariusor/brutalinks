package app

import pub "github.com/go-ap/activitypub"

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
