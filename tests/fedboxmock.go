package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	pub "github.com/go-ap/activitypub"
)

var root = fedbox{
	service: ap("http://localhost:6667").Type(pub.ServiceType),
	collections: map[string]*builder{
		"actors": ap("http://localhost:6667/actors").
			Type(pub.OrderedCollectionType).
			Items(
				ap("http://localhost:6667/actors/1").Type(pub.PersonType),
				ap("http://localhost:6667/actors/2").Type(pub.PersonType),
				ap("http://localhost:6667/actors/3").Type(pub.PersonType),
			),
		"activities": ap("http://localhost:6667/activities").
			Type(pub.OrderedCollectionType).
			Items(
				ap("http://localhost:6667/activities/1").Type(pub.CreateType),
			),
		"objects": ap("http://localhost:6667/objects").
			Type(pub.OrderedCollectionType).
			Items(
				ap("http://localhost:6667/objects/1").Type(pub.NoteType),
			),
	},
}

type fedbox struct {
	service     *builder
	collections map[string]*builder
}

func apiMockURL() string {
	listen := "localhost:6667"
	go http.ListenAndServe(listen, root)
	time.Sleep(time.Second)
	return fmt.Sprintf("http://%s", listen)
}

var validFedboxCollections = []string{"actors", "activities", "objects"}
var validObjectCollections = []string{"shares", "likes"}
var validActorCollections = []string{"inbox", "outbox", "followers", "following", "liked"}

func contains[T ~string](sl []T, el T) bool {
	for _, c := range sl {
		if strings.ToLower(string(c)) == strings.ToLower(string(el)) {
			return true
		}
	}
	return false
}

func readJson(it pub.Item) io.ReadSeeker {
	data, _ := pub.MarshalJSON(it)
	return bytes.NewReader(data)
}

func (f fedbox) content(name string) (time.Time, io.ReadSeeker) {
	dir, base := path.Split(name)
	if base == "" && dir == "/" {
		s := f.service.Build()
		return time.Now(), readJson(s)
	}
	if contains(validFedboxCollections, base) {
		return time.Now(), readJson(f.collections[base].Build())
	}
	if contains(validObjectCollections, base) {
		return time.Now(), readJson(pub.OrderedCollectionPage{})
	}
	if contains(validActorCollections, base) {
		return time.Now(), readJson(pub.OrderedCollectionPage{})
	}
	//localhost
	return time.Now(), bytes.NewReader([]byte{})
}

func (f fedbox) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path
	modTime, content := f.content(r.URL.Path)
	http.ServeContent(w, r, name, modTime, content)
}
