//go:generate go run -tags=dev assets_generate.go

package frontend

import (
	"net/http"
	"os"
	"path"
)

// Assets contains project assets.
var Assets = statics{}

type statics struct{}

func (s statics) Open(name string) (http.File, error) {
	file := path.Base(name)
	basename := path.Dir(name)
	switch basename {
	case "assets":
		fallthrough
	case "templates":
		fallthrough
	case "db":
		fallthrough
	case "docs":
		return http.Dir(basename).Open(file)
	}
	return nil, os.ErrNotExist
}
