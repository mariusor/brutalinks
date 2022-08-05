package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	TemplateDir = "templates/"
	AssetsDir   = "assets/"
)

const (
	year = 8766 * time.Hour
)

type AssetFiles map[string][]string
type AssetContents map[string][]byte

func (a AssetFiles) SubresourceIntegrityHash(name string) (string, bool) {
	files, ok := a[name]
	if !ok {
		return "", false
	}
	buf := new(bytes.Buffer)
	for _, asset := range files {
		ext := path.Ext(name)
		if len(ext) <= 1 {
			continue
		}
		dat, err := getFileContent(assetPath(asset))
		if err != nil {
			continue
		}
		buf.Write(dat)
	}
	b := buf.Bytes()
	if len(b) == 0 {
		return "", false
	}
	return sha(b), true
}

// GetFullFile
func GetFullFile(name string) ([]byte, error) {
	return getFileContent(name)
}

// TemplateNames returns asset names necessary for unrolled.Render
func TemplateNames() []string {
	names := make([]string, 0)
	fs.WalkDir(assets, TemplateDir, func(path string, info fs.DirEntry, err error) error {
		if info != nil && !info.IsDir() {
			names = append(names, path)
		}
		return nil
	})
	return names
}

var m sync.Mutex

func getFileContent(name string) ([]byte, error) {
	m.Lock()
	defer m.Unlock()

	f, err := openFsFn(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	b := make([]byte, fi.Size())
	if _, err = f.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func assetPath(pieces ...string) string {
	return path.Clean(path.Join(AssetsDir, path.Join(pieces...)))
}

// Svg returns an svg by path for display inside templates
func Svg(name string) template.HTML {
	return Asset()(name)
}

// Style returns a style by path for displaying inline
func Style(name string) template.CSS {
	return template.CSS(Asset()("css/" + name))
}

// Js returns a javascript file by path for displaying inline
func Js(name string) template.HTML {
	return Asset()("js/" + name)
}

// Template returns an asset by path for unrolled.Render
func Template(name string) ([]byte, error) {
	return getFileContent(name)
}

func sha(d []byte) string {
	sha := sha256.Sum256(d)
	return base64.StdEncoding.EncodeToString(sha[:])
}

func AssetSha(name string) string {
	dat, err := getFileContent(assetPath(name))
	if err != nil || len(dat) == 0 {
		return ""
	}
	return sha(dat)
}

// Integrity gives us the integrity attribute for Subresource Integrity
func Integrity(name string) template.HTMLAttr {
	return template.HTMLAttr(fmt.Sprintf(` identity="sha256-%s"`, AssetSha(name)))
}

func ServeAsset(s AssetFiles) func(w http.ResponseWriter, r *http.Request) {
	return writeAsset(s)
}

func ServeStatic(st string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		fullPath := filepath.Clean(filepath.Join(st, chi.URLParam(r, "path")))
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, immutable", int(year.Seconds())))
		http.ServeFile(w, r, fullPath)
	}
}

// Asset returns an asset by path for display inside templates
// it is mainly used for rendering the svg icons file
var Asset = assetLoad
