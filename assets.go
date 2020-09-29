//+build !dev
//go:generate go run -tags $(ENV) assets.go -build "prod qa" -src ./templates,./assets,./README.md -var assets -o internal/assets/assets.gen.go

package main

import (
	"aletheia.icu/broccoli/fs"
	"bytes"
	"flag"
	"fmt"
	ignore "github.com/sabhiram/go-gitignore"
	"github.com/tdewolff/minify"
	"github.com/tdewolff/minify/css"
	"github.com/tdewolff/minify/js"
	"github.com/tdewolff/minify/svg"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/ioutil"
	"log"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Generator collects the necessary info about the package and
// bundles the provided assets according to provided flags.
type Generator struct {
	pkg *Package

	inputFiles   []string // list of source dirs
	includeGlob  string   // files to be included
	excludeGlob  string   // files to be excluded
	useGitignore bool     // .gitignore files will be parsed
	quality      int      // compression level (1-11)
}

const template = `%s
package %s

import "aletheia.icu/broccoli/fs"

var %s = fs.New(%t, []byte(%q))
`

type wildcards []wildcard

func (w wildcards) test(path string, info os.FileInfo) bool {
	for _, card := range w {
		if !card.test(info) {
			if *verbose {
				log.Println("ignoring", path)
			}
			return false
		}
	}

	return true
}

func (g *Generator) generate() ([]byte, error) {
	var (
		files []*fs.File
		cards wildcards
		state = map[string]bool{}

		total int64
	)

	m := minify.New()
	m.AddFunc("image/svg+xml", svg.Minify)
	m.AddFunc("text/css", css.Minify)
	m.AddFuncRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), js.Minify)

	if g.includeGlob != "" {
		cards = append(cards, wildcardFrom(true, g.includeGlob))
	} else if g.excludeGlob != "" {
		cards = append(cards, wildcardFrom(false, g.excludeGlob))
	}

	if g.useGitignore {
		ignores, err := g.parseGitignores()
		if err != nil {
			return nil, fmt.Errorf("cannot open .gitignore: %w", err)
		}
		cards = append(cards, ignores...)
	}

	for _, input := range g.inputFiles {
		info, err := os.Stat(input)
		if err != nil {
			return nil, fmt.Errorf("file or directory %s not found", input)
		}

		var f *fs.File
		if !info.IsDir() {
			if _, ok := state[input]; ok {
				return nil, fmt.Errorf("duplicate path in the input: %s", input)
			}
			state[input] = true

			f, err = fs.NewFile(input)
			if err != nil {
				return nil, fmt.Errorf("cannot open file or directory: %w", err)
			}
			ext := filepath.Ext(input)
			if ext == ".css" || ext == ".js" || ext == ".svg" {
				o := bytes.Buffer{}
				m.Minify(mime.TypeByExtension(ext), &o, bytes.NewBuffer(f.Data))
				f.Data = o.Bytes()
				f.Fsize = int64(o.Len())
			}

			total += f.Fsize
			files = append(files, f)
			continue
		}

		err = filepath.Walk(input, func(path string, info os.FileInfo, _ error) error {
			if !cards.test(path, info) {
				return nil
			}

			f, err := fs.NewFile(path)
			if err != nil {
				return err
			}
			if _, ok := state[path]; ok {
				return fmt.Errorf("duplicate path in the input: %s", path)
			}
			ext := filepath.Ext(path)
			if ext == ".css" || ext == ".js" || ext == ".svg" {
				o := bytes.Buffer{}
				m.Minify(mime.TypeByExtension(ext), &o, bytes.NewBuffer(f.Data))
				f.Data = o.Bytes()
				f.Fsize = int64(o.Len())
			}

			total += f.Fsize
			state[path] = true
			files = append(files, f)
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("cannot open file or directory: %w", err)
		}
	}

	if *verbose {
		log.Println("total bytes read:", total)
	}

	bundle, err := fs.Pack(files, g.quality)
	if err != nil {
		return nil, fmt.Errorf("could not compress the input: %w", err)
	}

	if *verbose {
		log.Println("total bytes compressed:", len(bundle))
	}

	return bundle, nil
}

type wildcard interface {
	test(os.FileInfo) bool
}

type includeWildcard struct {
	include  bool
	patterns []string
}

func (w includeWildcard) test(info os.FileInfo) bool {
	if info.IsDir() {
		return true
	}

	pass := !w.include
	for _, pattern := range w.patterns {
		match, err := filepath.Match(pattern, info.Name())
		if err != nil {
			log.Fatal("invalid wildcard:", pattern)
		}

		if match {
			pass = w.include
			break
		}
	}

	return pass
}

func wildcardFrom(include bool, patterns string) wildcard {
	w := strings.Split(patterns, ",")
	for i, v := range w {
		w[i] = strings.Trim(v, ` "`)
	}

	return includeWildcard{include, w}
}

type gitignoreWildcard struct {
	ign *ignore.GitIgnore
}

func (w gitignoreWildcard) test(info os.FileInfo) bool {
	return !w.ign.MatchesPath(info.Name())
}

func (g *Generator) parseGitignores() (cards []wildcard, err error) {
	err = filepath.Walk(".", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && info.Name() == ".gitignore" {
			ign, err := ignore.CompileIgnoreFile(path)
			if err != nil {
				return err
			}
			cards = append(cards, gitignoreWildcard{ign: ign})
		}
		return nil
	})
	return
}

// Package holds information about a Go package
type Package struct {
	dir      string
	name     string
	defs     map[*ast.Ident]types.Object
	typesPkg *types.Package
}

func (g *Generator) parsePackage() {
	pkg, err := build.Default.ImportDir(".", 0)
	if err != nil {
		log.Fatalln("cannot parse package:", err)
	}

	var names []string
	names = append(names, pkg.GoFiles...)
	names = append(names, pkg.CgoFiles...)
	names = append(names, pkg.SFiles...)

	var astFiles []*ast.File
	g.pkg = new(Package)
	set := token.NewFileSet()
	for _, name := range names {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		parsedFile, err := parser.ParseFile(set, name, nil, parser.ParseComments)
		if err != nil {
			log.Fatalf("parsing package: %s: %s\n", name, err)
		}
		astFiles = append(astFiles, parsedFile)
	}
	if len(astFiles) == 0 {
		log.Fatalln("no buildable Go files")
	}
	g.pkg.name = astFiles[0].Name.Name
	g.pkg.dir = "."

	// Type check the package.
	g.pkg.check(set, astFiles)
}

// check type-checks the package.
func (pkg *Package) check(fs *token.FileSet, astFiles []*ast.File) {
	pkg.defs = make(map[*ast.Ident]types.Object)
	config := types.Config{Importer: defaultImporter(), FakeImportC: true}
	info := &types.Info{
		Defs: pkg.defs,
	}
	typesPkg, err := config.Check(pkg.dir, fs, astFiles, info)
	if err != nil {
		log.Println("checking package:", err)
		log.Println("proceeding anyway...")
	}

	pkg.typesPkg = typesPkg
}

func defaultImporter() types.Importer {
	return importer.For("source", nil)
}

var (
	flagInput       = flag.String("src", "public", "")
	flagOutput      = flag.String("o", "", "")
	flagVariable    = flag.String("var", "br", "")
	flagInclude     = flag.String("include", "", "")
	flagExclude     = flag.String("exclude", "", "")
	flagBuild       = flag.String("build", "", "")
	flagOptional    = flag.Bool("opt", false, "")
	flagGitignore   = flag.Bool("gitignore", false, "")
	flagQuality     = flag.Int("quality", 11, "")
	flagPackageName = flag.String("package", "assets", "")

	verbose = flag.Bool("v", false, "")
)

const (
	constInput = "public"
)

const help = `Usage: broccoli [options]

Broccoli uses brotli compression to embed a virtual file system in Go executables.

Options:
	-src folder[,file,file2]
		The input files and directories, "public" by default.
	-o
		Name of the generated file, follows input by default.
	-var br
		Name of the exposed variable, "br" by default.
	-include *.html,*.css
		Wildcard for the files to include, no default.
	-exclude *.wasm
		Wildcard for the files to exclude, no default.
	-build "linux,386 darwin,!cgo"
		Compiler build tags for the generated file, none by default.
	-package "assets"
		The package for the generated file.
	-opt
		Optional decompression: if enabled, files will only be decompressed
		on the first time they are read.
	-gitignore
		Enables .gitignore rules parsing in each directory, disabled by default.
	-quality [level]
		Brotli compression level (1-11), the highest by default.

Generate a broccoli.gen.go file with the variable broccoli:
	//go:generate broccoli -src assets -o broccoli -var broccoli

Generate a regular public.gen.go file, but include all *.wasm files:
	//go:generate broccoli -src public -include="*.wasm"`

var goIdentifier = regexp.MustCompile(`^\p{L}[\p{L}0-9_]*$`)

func main() {
	log.SetFlags(0)
	log.SetPrefix("broccoli: ")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, help)
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

	includeGlob := *flagInclude
	excludeGlob := *flagExclude
	if includeGlob != "" && excludeGlob != "" {
		log.Fatal("mutually exclusive options -include and -exclude found")
	}

	quality := *flagQuality
	if quality < 1 || quality > 11 {
		log.Fatalf("unsupported compression level %d (1-11)\n", quality)
	}

	g := Generator{
		inputFiles:   inputs,
		includeGlob:  includeGlob,
		excludeGlob:  excludeGlob,
		useGitignore: *flagGitignore,
		quality:      quality,
	}

	g.parsePackage()

	bundle, err := g.generate()
	if err != nil {
		log.Fatal(err)
	}

	header := "// Code generated by broccoli at %v."
	header = fmt.Sprintf(header, time.Now().Format(time.RFC3339))

	buildTags := *flagBuild
	if buildTags != "" {
		header = "// +build " + buildTags + "\n\n" + header
	}

	code := fmt.Sprintf(template,
		header, *flagPackageName, variable, *flagOptional, bundle)

	err = ioutil.WriteFile(output, []byte(code), 0644)
	if err != nil {
		log.Fatalf("could not write to %s: %v\n", output, err)
	}
}
