package brutalinks

import (
	"errors"
	"sort"

	"github.com/RoaringBitmap/roaring/roaring64"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/filters"
	"github.com/go-ap/filters/index"
)

func (r *repository) Search(checks ...filters.Check) (vocab.ItemCollection, error) {
	return r.st.Search(checks...)
}

func (r *repository) SearchInCollection(col vocab.IRI, checks ...filters.Check) (vocab.ItemCollection, error) {
	return r.st.SearchInCollection(col, checks...)
}

var SortFn = vocab.ItemOrderTimestamp

func (st *Storage) searchBitmaps(bmp *roaring64.Bitmap, checks ...filters.Check) (vocab.ItemCollection, error) {
	if bmp.IsEmpty() {
		return nil, nil
	}

	errs := make([]error, 0)
	result := make(vocab.ItemCollection, 0, bmp.GetCardinality())

	filter := filters.FilterChecks(checks...)

	iter := bmp.Iterator()
	for iter.HasNext() {
		ref := iter.Next()
		li, err := st.loadFromRef(ref)
		if err != nil {
			errs = append(errs, err)
		}
		if it, ok := li.(vocab.Item); ok {
			if it = filter.Run(it); it != nil {
				result = append(result, it)
			}
		}
	}

	if SortFn != nil {
		sort.Slice(result, func(i, j int) bool {
			return SortFn(result[i], result[j])
		})
	}
	if cursorChecks := filters.PaginationChecks(checks...); len(cursorChecks) > 0 {
		if paginated, ok := filters.PaginateCollection(result, cursorChecks...).(vocab.ItemCollection); ok {
			result = paginated
		}
	}

	if maxCnt := filters.MaxCount(checks...); maxCnt > 0 && maxCnt < len(result) {
		result = result[:maxCnt]
	}

	return result, errors.Join(errs...)
}

func (st *Storage) Search(checks ...filters.Check) (vocab.ItemCollection, error) {
	bmp := filters.Checks(checks).IndexMatch(st.in)
	if bmp.IsEmpty() {
		return nil, nil
	}

	return st.searchBitmaps(bmp, checks...)
}

func (st *Storage) SearchInCollection(col vocab.IRI, checks ...filters.Check) (vocab.ItemCollection, error) {
	bmp := roaring64.FastOr(index.GetBitmaps[uint64](st.in[ByCollection], index.HashFn(col))...)
	if bmp.IsEmpty() {
		return nil, nil
	}

	bmp.And(filters.Checks(checks).IndexMatch(st.in))
	if bmp.IsEmpty() {
		return nil, nil
	}

	return st.searchBitmaps(bmp, checks...)
}
