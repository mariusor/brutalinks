//+build dev

package assets

import (
	"os"
	"path/filepath"
)

var walkFsFn = filepath.Walk
var openFsFn = os.Open
