package server

import (
	"io/fs"
	"os"
)

// openAPIFS reads OpenAPI specs from the docs/api directory at runtime.
var openAPIFS fs.FS = os.DirFS("docs/api")
