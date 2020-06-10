package assets

import (
	"bufio"
	"bytes"
	"html/template"
	"os"
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

// Template returns an asset by path for unrolled.Render
func Template(name string) ([]byte, error) {
	return getFileContent(name)
}
