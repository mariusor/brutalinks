package main

import (
	"fmt"
	pub "github.com/go-ap/activitypub"
	"testing"
)

func TestFedbox_resolve(t *testing.T) {
	f := newFedBOX("localhost:1111")
	root, _ := f.resolve("/", nil)
	if root.GetType() != pub.ServiceType {
		t.Errorf("Invalid type for root node: %q, expected %q", root.GetType(), pub.ServiceType)
	}
	colName := "actors"
	actors, _ := f.resolve(fmt.Sprintf("/%s", colName), nil)
	if actors.GetType() != pub.OrderedCollectionType {
		t.Errorf("Invalid type for %s collection node: %q, expected %q", colName, actors.GetType(), pub.OrderedCollectionType)
	}
	expectedCount := uint(len(f.collections[colName].items))
	pub.OnCollectionIntf(actors, func(col pub.CollectionInterface) error {
		if col.Count() != expectedCount {
			t.Errorf("Invalid count for %s node: %d, expected %d", colName, col.Count(), expectedCount)
		}
		return nil
	})
}
