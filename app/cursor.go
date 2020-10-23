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