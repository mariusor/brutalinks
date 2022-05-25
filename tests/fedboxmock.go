package main

import (
	"context"
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	authfs "github.com/go-ap/auth/fs"
	fb "github.com/go-ap/fedbox/app"
	"github.com/go-ap/fedbox/storage/fs"
	"github.com/sirupsen/logrus"
)

func apiMockURL(t *testing.T) string {
	listen := "127.0.0.1:6667"
	cwd, _ := os.Getwd()
	mockPath := path.Join(cwd, "mocks")

	os.Setenv("HTTPS", "false")
	os.Setenv("HOSTNAME", listen)
	os.Setenv("LISTEN", listen)
	os.Setenv("LOG_OUTPUT", "json")
	os.Setenv("FEDBOX_DISABLE_CACHE", "true")
	os.Setenv("STORAGE_PATH", mockPath)
	c, err := fb.Config("test", time.Second)
	if err != nil {
		t.Errorf("unable to initialize FedBOX config: %s", err)
		os.Exit(1)
	}
	r, err := fs.New(fs.Config{StoragePath: c.StoragePath, BaseURL: listen})
	if err != nil {
		t.Errorf("unable to initialize FedBOX storage: %s", err)
		os.Exit(1)
	}
	or := authfs.New(authfs.Config{Path: c.StoragePath})

	f, err := fb.New(logrus.New(), runtime.Version(), c, r, or)
	if err != nil {
		t.Errorf("unable to initialize FedBOX: %s", err)
		os.Exit(1)
	}
	go f.Run(context.TODO())
	time.Sleep(500 * time.Millisecond)
	return f.Config().BaseURL
}
