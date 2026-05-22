package api

import _ "embed"

// OpenAPI is the embedded OpenAPI 3.0 specification.
//
//go:embed openapi.yaml
var OpenAPI []byte
