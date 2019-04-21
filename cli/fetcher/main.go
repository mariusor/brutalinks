package main

import (
	"flag"
	"github.com/go-ap/activitystreams"
	"github.com/mariusor/littr.go/app/api"
	"github.com/mariusor/littr.go/internal/log"
	"io/ioutil"
	"net/http"
	"os"
)

func validContentType(c ...string) bool {
	for _, ct := range c {
		switch ct {
		case "application/json":
			fallthrough
		case "application/ld+json":
			fallthrough
		case "application/activity+json":
			fallthrough
		case "application/activity+json; charset=utf-8":
			return true
		}
	}
	return false
}

func main() {
	var url string
	flag.StringVar(&url, "url", "", "the URL that we should fetch")
	flag.Parse()
	var err error
	log := log.Dev(log.TraceLevel)

	if url == "" {
		log.Error("you need to pass a url value to fetch")
		os.Exit(1)
	}

	r := api.New(api.Config{Logger: log})
	log.Infof("fetching %s", url)

	var resp *http.Response
	if resp, err = r.Get(url); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	if resp == nil {
		log.Error("nil response from the repository")
		os.Exit(1)
	}
	if resp.StatusCode != http.StatusOK {
		log.Error("unable to load from the API")
		os.Exit(1)
	}
	if !validContentType(resp.Header["Content-Type"]...) {
		log.Errorf("mismatched content-type of uri payload %v", resp.Header["Content-Type"])
		os.Exit(1)
	}
	defer resp.Body.Close()
	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	if o, err := activitystreams.UnmarshalJSON(body); err == nil {
		log.Infof("received %#v", o)
	}

	log.Info(string(body))
}
