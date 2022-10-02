//go:build prod || qa

package assets

import (
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"time"
)

func Write(s fs.FS) func(http.ResponseWriter, *http.Request) {
	const cacheTime = 8766 * time.Hour

	assetContents := make(map[string][]byte)
	mime.AddExtensionType(".ico", "image/vnd.microsoft.icon")
	mime.AddExtensionType(".text", "text/plain; charset=utf-8")
	return func(w http.ResponseWriter, r *http.Request) {
		asset := r.RequestURI

		mimeType := mime.TypeByExtension(filepath.Ext(asset))
		if asset == "/ns" {
			mimeType = "application/xrd+json; charset=utf-8"
		}
		buf, ok := assetContents[asset]
		if !ok {
			cont, err := fs.ReadFile(s, asset)
			if err != nil {
				w.Write([]byte(fmt.Sprintf("not found: %s", err)))
				w.WriteHeader(http.StatusNotFound)
				return
			}
			buf = cont
		}
		assetContents[asset] = buf

		w.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", int(cacheTime.Seconds())))
		w.Header().Set("Content-Type", mimeType)
		w.Write(buf)
	}
}
