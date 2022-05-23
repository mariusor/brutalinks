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

var service = ap(pub.ServiceType).ID("https://localhost:6667")
var fedboxCollections = map[string]*builder{
	"actors":     ap(pub.OrderedCollectionType).ID("https://localhost:6667/actors"),
	"activities": ap(pub.OrderedCollectionType).ID("https://localhost:6667/activities"),
	"objects":    ap(pub.OrderedCollectionType).ID("https://localhost:6667/objects"),
}

type fedbox struct{}

func apiMockURL() string {
	listen := "localhost:6667"
	m := fedbox{}
	go http.ListenAndServe(listen, m)
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

func content(name string) (time.Time, io.ReadSeeker) {
	dir, base := path.Split(name)
	if base == "" && dir == "/" {
		s := service.Build()
		return time.Now(), readJson(s)
	}
	if contains(validFedboxCollections, base) {
		return time.Now(), readJson(fedboxCollections[base].Build())
	}
	if contains(validObjectCollections, base) {
		return time.Now(), readJson(pub.OrderedCollectionPage{})
	}
	if contains(validActorCollections, base) {
		return time.Now(), readJson(pub.OrderedCollectionPage{})
	}

	return time.Now(), bytes.NewReader([]byte{})
}

func (f fedbox) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path
	modTime, content := content(r.URL.Path)
	http.ServeContent(w, r, name, modTime, content)
}
