package brutalinks

import (
	"sort"
	"time"

	vocab "github.com/go-ap/activitypub"
)

type Cursor struct {
	after  vocab.IRI
	before vocab.IRI
	items  RenderableList
	total  uint
}

var emptyCursor = Cursor{}

type RenderableList []Renderable

func (r *RenderableList) Valid() bool {
	return r != nil && len(*r) > 0
}

func (r RenderableList) Items() ItemCollection {
	items := make(ItemCollection, 0)
	for _, ren := range r {
		if it, ok := ren.(*Item); ok {
			items = append(items, *it)
		}
	}
	return items
}

func (r RenderableList) Contains(ren Renderable) bool {
	for _, rr := range r {
		if rr.Type() == ren.Type() && rr.ID() == ren.ID() {
			return true
		}
	}
	return false
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
		*r = append(*r, o)
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
	if !Instance.Conf.VotingEnabled {
		return ByDate(r)
	}
	rl := make([]Renderable, 0, len(r))
	for _, rr := range r {
		rl = append(rl, rr)
	}
	sort.SliceStable(rl, func(i, j int) bool {
		var hi, hj float64

		ri := rl[i]
		ii, oki := ri.(*Item)

		rj := rl[j]
		ij, okj := rj.(*Item)
		if oki {
			hi = Hacker(int64(ii.Votes.Score()), time.Now().Sub(ii.SubmittedAt))
		}
		if okj {
			hj = Hacker(int64(ij.Votes.Score()), time.Now().Sub(ij.SubmittedAt))
		}
		if oki && okj && hi+hj > 0 {
			return hi >= hj
		}
		return ri.Date().After(rj.Date())
	})
	return rl
}

func lastUpdatedInThread(it Renderable) time.Time {
	maxDate := it.Date()
	ob, ok := it.(*Item)
	if !ok {
		return maxDate
	}
	for _, ic := range *ob.Children() {
		if threadLastUpdate := lastUpdatedInThread(ic); threadLastUpdate.After(maxDate) {
			maxDate = threadLastUpdate
		}
	}
	return maxDate
}

func ByRecentActivity(r RenderableList) []Renderable {
	rl := make([]Renderable, 0, len(r))
	for _, rr := range r {
		rl = append(rl, rr)
	}
	sort.SliceStable(rl, func(i, j int) bool {
		ri := rl[i]
		rj := rl[j]
		return lastUpdatedInThread(ri).After(lastUpdatedInThread(rj))
	})
	return rl
}
