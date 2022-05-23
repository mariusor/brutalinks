package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-ap/errors"
	"github.com/go-ap/handlers"
	"github.com/go-ap/jsonld"

	pub "github.com/go-ap/activitypub"
)

func langRef(s string) pub.LangRefValue {
	return pub.LangRefValue{
		Ref:   pub.NilLangRef,
		Value: pub.Content(s),
	}
}

type fedbox struct {
	base        pub.IRI
	service     *builder
	collections map[string]*builder
}

func newFedBOX(listen string) *fedbox {
	base := pub.IRI(fmt.Sprintf("http://%s", listen))
	baseIRI := func(s string) pub.IRI {
		return base.AddPath(s)
	}
	return &fedbox{
		base:    base,
		service: ap(base).Type(pub.ServiceType),
		collections: map[string]*builder{
			"actors": ap(baseIRI("actors")).
				Type(pub.OrderedCollectionType).
				Items(
					ap(baseIRI("actors/1")).Type(pub.PersonType).Name(langRef("marius")),
					ap(baseIRI("actors/2")).Type(pub.PersonType).Name(langRef("admin")),
					ap(baseIRI("actors/3")).Type(pub.PersonType).Name(langRef("rwar")),
				),
			"activities": ap(baseIRI("activities")).
				Type(pub.OrderedCollectionType).
				Items(
					ap(baseIRI("activities/1")).Type(pub.CreateType),
				),
			"objects": ap(baseIRI("objects")).
				Type(pub.OrderedCollectionType).
				Items(
					ap(baseIRI("objects/1")).Type(pub.NoteType),
				),
		},
	}
}

func apiMockURL() string {
	listen := "127.0.0.1:6667"
	f := newFedBOX(listen)
	go http.ListenAndServe(listen, f)
	time.Sleep(time.Second)
	return f.service.Build().GetLink().String()
}

var validFedBOXCollections = []string{"actors", "activities", "objects"}
var validActivityPubCollections = []string{"shares", "likes", "inbox", "outbox", "followers", "following", "liked"}

func contains[T ~string](sl []T, el T) bool {
	for _, c := range sl {
		if strings.ToLower(string(c)) == strings.ToLower(string(el)) {
			return true
		}
	}
	return false
}

func errJson(err error) io.ReadSeeker {
	b, _ := jsonld.Marshal(err)
	return bytes.NewReader(b)
}

func readJson(it pub.Item) io.ReadSeeker {
	data, _ := pub.MarshalJSON(it)
	return bytes.NewReader(data)
}

func (f fedbox) resolve(name string) pub.Item {
	dir, base := path.Split(name)
	if base == "" && dir == "/" {
		return f.service.Build()
	}
	if contains(validFedBOXCollections, base) {
		return f.collections[base]
	} else {
		// name = "actors/1/inbox -> dir=actors/1 base=inbox
		for _, collection := range f.collections {
			for _, it := range collection.items {
				if it.GetID().String() == dir {
					if contains(validActivityPubCollections, base) {
						// it's an object's collection
						return handlers.CollectionType(base).Of(it)
					} else {
						// it's an object
						return it
					}
				}
			}
		}
	}
	return nil
}

func (f fedbox) content(name string) (time.Time, io.ReadSeeker) {
	it := f.resolve(name)
	if it == nil {
		return time.Time{}, errJson(errors.NotFoundf("%s not found", name))
	}
	mod := time.Now()
	pub.OnObject(it, func(ob *pub.Object) error {
		if !ob.Published.IsZero() {
			mod = ob.Published
		}
		if !ob.Updated.IsZero() {
			mod = ob.Updated
		}
		return nil
	})
	return mod, readJson(it)
}

func (f fedbox) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path
	modTime, content := f.content(r.URL.Path)
	http.ServeContent(w, r, name, modTime, content)
}
