//go:build !(prod || qa)

package assets

import (
	"os"
	"path/filepath"

	assetFS "git.sr.ht/~mariusor/assets/fs"
)

var assetDir, _ = filepath.Abs("./assets")
var readme, _ = filepath.Abs("./README.md")
var AssetFS = assetFS.Aggregate(
	os.DirFS(assetDir),
	os.DirFS(readme),
)

var templateDir, _ = filepath.Abs("./")
var TemplateFS = os.DirFS(templateDir)
