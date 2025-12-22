package apidocs

import "embed"

// FS holds embedded OpenAPI specifications served by the API server.
//
//go:embed *.yaml
var FS embed.FS
