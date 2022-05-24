package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-ap/handlers"
	"github.com/go-ap/jsonld"
	"github.com/mariusor/go-littr/app"
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
					ap(baseIRI("actors/mock-app-1")).Type(pub.ApplicationType).Name(langRef("mock-app")),
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

func (f fedbox) resolve(name string, ff *app.Filters) (pub.Item, error) {
	dir, base := path.Split(name)
	if base == "" && dir == "/" {
		return f.service.Build(), nil
	}
	if contains(validFedBOXCollections, base) {
		return f.collections[base].Build(), nil
	} else {
		// name = "actors/1/inbox -> dir=actors/1 base=inbox
		for _, collection := range f.collections {
			for _, it := range collection.items {
				ui, _ := it.GetID().URL()
				if ui.Path == name {
					if contains(validActivityPubCollections, base) {
						// it's an object's collection
						return handlers.CollectionType(base).Of(it), nil
					} else {
						// it's an object builder
						return it.Build(), nil
					}
				}
			}
		}
	}
	return nil, errors.NotFoundf("%s not found", name)
}

func (f fedbox) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path
	var content []byte

	status := http.StatusOK
	if strings.Contains(name, "oauth") {
		t := struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
		}{
			AccessToken: "ok",
			TokenType:   "huh?",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		content, _ = json.Marshal(t)
		w.Write(content)
		return
	}
	it, err := f.resolve(name, app.FiltersFromRequest(r))
	if err != nil {
		status = errors.HttpStatus(err)
		content, _ = jsonld.Marshal(it)
		w.WriteHeader(status)
		w.Write(content)
		return
	}

	if content, err = jsonld.Marshal(it); err != nil {
		status = errors.HttpStatus(err)
		content, _ = jsonld.Marshal(it)
	}
	w.WriteHeader(status)
	w.Write(content)
}
