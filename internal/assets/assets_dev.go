//go:build !(prod || qa)

package assets

import (
	"fmt"
	assetFS "git.sr.ht/~mariusor/assets/fs"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"
)

var (
	assetDir, _ = filepath.Abs("./assets")
	readme, _   = filepath.Abs("./README.md")
	AssetFS     = assetFS.Aggregate(os.DirFS(assetDir), os.DirFS(readme))

	templateDir, _ = filepath.Abs("./")
	TemplateFS     = os.DirFS(templateDir)
)

func Write(s fs.FS) func(http.ResponseWriter, *http.Request) {
	const cacheTime = 8766 * time.Hour
	return func(w http.ResponseWriter, r *http.Request) {
		asset := r.RequestURI
		mimeType := mime.TypeByExtension(path.Ext(r.RequestURI))

		buf, err := fs.ReadFile(s, asset)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("not found: %s", err)))
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", int(cacheTime.Seconds())))
		w.Header().Set("Content-Type", mimeType)
		w.Write(buf)
	}
}
