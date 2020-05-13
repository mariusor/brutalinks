package assets

import (
	"bufio"
	"bytes"
	"github.com/tdewolff/minify"
	"github.com/tdewolff/minify/css"
	"github.com/tdewolff/minify/html"
	"github.com/tdewolff/minify/js"
	"github.com/tdewolff/minify/svg"
	"html/template"
	"os"
	"path"
	"regexp"
)

const (
	templateDir = "templates/"
	assetsDir   = "assets/"
)

// Files returns asset names necessary for unrolled.Render
func Files() []string {
	names := make([]string, 0)
	walkFsFn(templateDir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
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


// Asset returns an asset by path for display inside templates
// it is mainly used for rendering the svg icons file
func Asset(mime string) func (string) template.HTML {
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

// Template returns an asset by path for unrolled.Render
func Template(name string) ([]byte, error) {
	return getFileContent(name)
}
