// +build prod qa

package assets

import (
	"bytes"
	"fmt"
	"github.com/go-chi/chi"
	"github.com/tdewolff/minify"
	"github.com/tdewolff/minify/css"
	"github.com/tdewolff/minify/html"
	"github.com/tdewolff/minify/js"
	"github.com/tdewolff/minify/svg"
	"html/template"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"time"
)

// generated with broccoli - see /assets.go
var walkFsFn = assets.Walk
var openFsFn = assets.Open

// Asset returns an asset by path for display inside templates
// it is mainly used for rendering the svg icons file
func Asset(mime string) func(string) template.HTML {
	m := minify.New()
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("image/svg+xml", svg.Minify)
	m.AddFunc("text/css", css.Minify)
	m.AddFuncRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), js.Minify)

	return func(name string) template.HTML {
		b, _ := getFileContent(path.Join(assetsDir, name))
		o := bytes.Buffer{}
		m.Minify(mime, &o, bytes.NewBuffer(b))
		return template.HTML(o.Bytes())
	}
}

const year = 8766 * time.Hour

func ServeStatic(st string) func(w http.ResponseWriter, r *http.Request) {
	m := minify.New()
	m.AddFunc("image/svg+xml", svg.Minify)
	m.AddFunc("text/css", css.Minify)
	m.AddFuncRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), js.Minify)

	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(chi.URLParam(r, "path"))
		fullPath := filepath.Join(st, path)

		mw := m.ResponseWriter(w, r)
		defer mw.Close()

		w = mw
		w.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", int(year.Seconds())))
		http.ServeFile(w, r, fullPath)
	}
}

func ServeAsset(s AssetFiles) func(w http.ResponseWriter, r *http.Request) {
	m := minify.New()
	m.AddFunc("image/svg+xml", svg.Minify)
	m.AddFunc("text/css", css.Minify)
	m.AddFuncRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), js.Minify)

	writeFn := func(mimeType string, w io.Writer, b []byte) {
		rdr := new(bytes.Buffer)
		rdr.Write(b)
		m.Minify(mimeType, w, rdr)
	}
	return writeAsset(s, writeFn)
}
