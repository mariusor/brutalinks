// +build !prod,!qa

package assets

import (
	"fmt"
	"github.com/go-chi/chi"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var walkFsFn = filepath.Walk
var openFsFn = os.Open

// Asset returns an asset by path for display inside templates
// it is mainly used for rendering the svg icons file
func Asset(mime string) func(string) template.HTML {
	return func(name string) template.HTML {
		b, _ := getFileContent(assetPath(name))
		return template.HTML(b)
	}
}

const year = 8766 * time.Hour

func ServeStatic(st string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(chi.URLParam(r, "path"))
		fullPath := filepath.Join(st, path)

		w.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", int(year.Seconds())))
		http.ServeFile(w, r, fullPath)
	}
}

func ServeAsset(s AssetFiles) func(w http.ResponseWriter, r *http.Request) {
	return writeAsset(s, nil)
}
