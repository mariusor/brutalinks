package assets

import (
	"bufio"
	"bytes"
	"html/template"
	"os"
	"path"
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

// Asset returns an asset by path for display inside templates
// it is mainly used for rendering the svg icons file
func Asset(name string) template.HTML {
	b, _ := getFileContent(path.Join(assetsDir, name))
	return template.HTML(b)
}

// Template returns an asset by path for unrolled.Render
func Template(name string) ([]byte, error) {
	return getFileContent(name)
}
