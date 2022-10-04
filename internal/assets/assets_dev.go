//go:build !(prod || qa)

package assets

import (
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"git.sr.ht/~mariusor/assets"
)

var (
	assetDir, _ = filepath.Abs("./assets")
	readme, _   = filepath.Abs("./README.md")
	AssetFS     = assets.Aggregate(os.DirFS(assetDir), os.DirFS(readme))

	templateDir, _ = filepath.Abs("./")
	TemplateFS     = os.DirFS(templateDir)
)

func Write(s fs.FS) func(http.ResponseWriter, *http.Request) {
	const cacheTime = 8766 * time.Hour

	mime.AddExtensionType(".ico", "image/vnd.microsoft.icon")
	mime.AddExtensionType(".txt", "text/plain; charset=utf-8")
	return func(w http.ResponseWriter, r *http.Request) {
		asset := r.RequestURI
		mimeType := mime.TypeByExtension(filepath.Ext(asset))

		buf, err := fs.ReadFile(s, asset)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("not found: %s", err)))
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", int(cacheTime.Seconds())))
		if mimeType != "" {
			w.Header().Set("Content-Type", mimeType)
		}
		w.Write(buf)
	}
}
