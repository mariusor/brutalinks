//go:build prod || qa

package assets

import (
	"bytes"
	"fmt"
	"html/template"
	"mime"
	"net/http"
	"path"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

// generated with the aletheia.icu/broccoli/fs package - see /assets.go
var walkFsFn = assets.Walk
var openFsFn = assets.Open

func writeAsset(s AssetFiles) func(http.ResponseWriter, *http.Request) {
	assetContents := make(AssetContents)
	return func(w http.ResponseWriter, r *http.Request) {
		asset := filepath.Clean(chi.URLParam(r, "path"))
		ext := path.Ext(r.RequestURI)
		mimeType := mime.TypeByExtension(ext)
		files, ok := s[asset]
		if !ok {
			w.Write([]byte("not found"))
			w.WriteHeader(http.StatusNotFound)
			return
		}

		cont, ok := assetContents[asset]
		if !ok {
			buf := bytes.Buffer{}
			for _, file := range files {
				if piece, _ := getFileContent(assetPath(file)); len(piece) > 0 {
					buf.Write(piece)
				}
			}
			assetContents[asset] = buf.Bytes()
		}
		cont = assetContents[asset]

		w.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", int(year.Seconds())))
		w.Header().Set("Content-Type", mimeType)
		w.Write(cont)
	}
}

func assetLoad() func(string) template.HTML {
	assetContents := make(AssetContents)
	return func(name string) template.HTML {
		cont, ok := assetContents[name]
		if !ok {
			cont, _ = getFileContent(assetPath(name))
			assetContents[name] = cont
		}
		return template.HTML(cont)
	}
}
