//go:build !dev

//go:generate go run -tags $(ENV) assets.go -build "prod || qa" -glob templates/*,templates/partials/*,templates/partials/*/* -var TemplateFS -o internal/assets/templates.gen.go
//go:generate go run -tags $(ENV) assets.go -build "prod || qa" -glob assets/*,assets/css/*,assets/js/*,README.md -var AssetFS -o internal/assets/assets.gen.go

package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	assets "git.sr.ht/~mariusor/assets/fs"
	"github.com/tdewolff/minify"
	"github.com/tdewolff/minify/css"
	"github.com/tdewolff/minify/js"
	"github.com/tdewolff/minify/svg"
)

var (
	flagInput       = flag.String("glob", "public/*", "")
	flagOutput      = flag.String("o", "", "")
	flagVariable    = flag.String("var", "br", "")
	flagBuild       = flag.String("build", "", "")
	flagGitignore   = flag.Bool("gitignore", false, "")
	flagPackageName = flag.String("package", "assets", "")
)

const (
	constInput = "assets"
)

const help = `Usage: minify [options]

Minify uses gzip compression and minification to embed a virtual file system in the Go executables.

Options:
	-glob folder/*[,folder1/*/*.ext]
		The glob paths to scan for files. It uses the path.Match patterns. Defaults to public/*
	-o
		Name of the generated file, follows input by default.
	-var assets
		Name of the exposed variable, "assets" by default.
	-build "linux,386 darwin,!cgo"
		Compiler build tags for the generated file, none by default.
	-package "assets"
		The package for the generated file.
	-gitignore
		Enables .gitignore rules parsing in each directory, disabled by default.

Generate a minify.gen.go file with the variable minify:
	//go:generate minify -glob assets/* -var minify
`

var goIdentifier = regexp.MustCompile(`^\p{L}[\p{L}0-9_]*$`)

func main() {
	log.SetFlags(0)
	log.SetPrefix("minify: ")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, help)
	}

	flag.Parse()
	if len(os.Args) <= 1 {
		flag.Usage()
		return
	}

	var inputs []string
	if flagInput == nil {
		inputs = []string{constInput}
	} else {
		inputs = strings.Split(*flagInput, ",")
	}

	output := *flagOutput
	if output == "" {
		output = strings.TrimLeft(inputs[0], "../")
	}
	if !strings.HasSuffix(output, ".gen.go") {
		output = strings.Split(output, ".")[0] + ".gen.go"
	}

	variable := *flagVariable
	if !goIdentifier.MatchString(variable) {
		log.Fatalln(variable, "is not a valid Go identifier")
	}

	stripAssetPrefix := func(f *assets.File) error {
		f.Fpath = strings.TrimPrefix(f.Fpath, "assets/")
		return nil
	}

	bundle, err := assets.Glob(inputs...).Pack(stripAssetPrefix, minifier().pack)
	if err != nil {
		log.Fatal(err)
	}

	code, err := assets.GenerateCode(*flagPackageName, variable, *flagBuild, bundle)
	if err != nil {
		log.Fatalf("could not buildFiles file: %v\n", err)
	}
	if err = os.WriteFile(output, code, 0644); err != nil {
		log.Fatalf("could not write to %s: %v\n", output, err)
	}
}

type m struct {
	*minify.M
}

func minifier() *m {
	m := new(m)
	m.M = minify.New()
	m.AddFunc("image/svg+xml", svg.Minify)
	m.AddFunc("text/css", css.Minify)
	m.AddFuncRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), js.Minify)
	return m
}

func (m *m) pack(f *assets.File) error {
	ext := filepath.Ext(f.Fpath)
	if !(ext == ".css" || ext == ".js" || ext == ".svg") {
		return nil
	}
	o := bytes.Buffer{}
	if err := m.Minify(mime.TypeByExtension(ext), &o, bytes.NewBuffer(f.Data)); err != nil {
		return err
	}
	log.Printf("minified file: %s", f.Fpath)
	f.Data = o.Bytes()
	f.Fsize = int64(o.Len())
	return nil
}
