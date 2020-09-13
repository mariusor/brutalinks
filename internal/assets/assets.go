package assets

import (
	"bufio"
	"bytes"
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
	r := bufio.NewReader(f)
	b := bytes.Buffer{}
	_, err = r.WriteTo(&b)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Svg returns an svg by path for display inside templates
func Svg(name string) template.HTML {
	return Asset("image/svg+xml")(name)
}

// Svg returns an svg by path for display inside templates
func Js(name string) template.HTML {
	return Asset("application/javascript")("js/" + name)
}

// Template returns an asset by path for unrolled.Render
func Template(name string) ([]byte, error) {
	return getFileContent(name)
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
			if cont, _ := getFileContent(filepath.Join(assetsDir, ext[1:], file)); len(cont) > 0 {
				writeFn(mimeType, w, cont)
			}
		}
	}
}
