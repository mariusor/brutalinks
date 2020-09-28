package assets

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"github.com/go-chi/chi"
	"html/template"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
)

const (
	templateDir = "templates/"
	assetsDir   = "assets/"
)

type AssetFiles map[string][]string

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
		dat, err := getFileContent(assetPath(ext[1:], asset))
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
	walkFsFn(templateDir, func(path string, info os.FileInfo, err error) error {
		if info != nil && !info.IsDir() {
			names = append(names, path)
		}
		return nil
	})
	return names
}

func getFileContent(name string) ([]byte, error) {
	f, err := openFsFn(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	b := bytes.Buffer{}
	_, err = r.WriteTo(&b)
	if err != nil {
		if err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

func assetPath(pieces ...string) string {
	return path.Clean(path.Join(assetsDir, path.Join(pieces...)))
}

// Svg returns an svg by path for display inside templates
func Svg(name string) template.HTML {
	return Asset("image/svg+xml")(name)
}

// Style returns a style by path for displaying inline
func Style(name string) template.CSS {
	return template.CSS(Asset("text/css")("css/" + name))
}

// Svg returns an svg by path for displaying inline
func Js(name string) template.HTML {
	return Asset("application/javascript")("js/" + name)
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

func writeAsset(s AssetFiles, writeFn func(s string, w io.Writer, b []byte)) func(http.ResponseWriter, *http.Request) {
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

		if writeFn == nil {
			writeFn = func(s string, w io.Writer, b []byte) {
				w.Write(b)
			}
		}

		w.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", int(year.Seconds())))
		w.Header().Set("Content-Type", mimeType)
		for _, file := range files {
			if cont, _ := getFileContent(assetPath(ext[1:], file)); len(cont) > 0 {
				writeFn(mimeType, w, cont)
			}
		}
	}
}
