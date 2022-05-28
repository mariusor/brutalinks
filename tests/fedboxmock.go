package main

import (
	"fmt"
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

const (
	fedboxHost = "127.0.0.1"
	fedboxPort = 6667
)

func mockFedBOX(t *testing.T) (*fb.FedBOX, error) {
	listen := fmt.Sprintf("%s:%d", fedboxHost, fedboxPort)
	cwd, _ := os.Getwd()
	mockPath := path.Join(cwd, "mocks")

	os.Setenv("HTTPS", "false")
	os.Setenv("HOSTNAME", listen)
	os.Setenv("LISTEN", listen)
	os.Setenv("LOG_OUTPUT", "text")
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
	or := authfs.New(authfs.Config{Path: path.Join(c.StoragePath, listen)})

	return fb.New(logrus.StandardLogger(), runtime.Version(), c, r, or)
}
