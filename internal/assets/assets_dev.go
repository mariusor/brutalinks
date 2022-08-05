//go:build !prod && !qa

package assets

import (
	"bytes"
	"fmt"
	"html/template"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

var local, _ = filepath.Abs(".")
var assets = os.DirFS(local)

func writeAsset(s AssetFiles) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		asset := filepath.Clean(chi.URLParam(r, "path"))
		if asset == "." {
			_, asset = filepath.Split(r.RequestURI)
		}
		mimeType := mime.TypeByExtension(path.Ext(r.RequestURI))

		files, ok := s[asset]
		if !ok {
			w.Write([]byte("not found"))
			w.WriteHeader(http.StatusNotFound)
			return
		}

		buf := bytes.Buffer{}
		for _, file := range files {
			if len(mimeType) == 0 {
				mimeType = mime.TypeByExtension(path.Ext(file))
			}
			if piece, _ := getFileContent(assetPath(file)); len(piece) > 0 {
				buf.Write(piece)
			}
		}

		w.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", int(year.Seconds())))
		w.Header().Set("Content-Type", mimeType)
		w.Write(buf.Bytes())
	}
}

func assetLoad() func(string) template.HTML {
	return func(name string) template.HTML {
		b, _ := getFileContent(assetPath(name))
		return template.HTML(b)
	}
}
