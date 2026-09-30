package axlr

import "github.com/google/jsonschema-go/jsonschema"

// compiledToolSchema retains numeric decision points alongside the resolved
// schema so argument conversion cannot round a value across an assertion.
type compiledToolSchema struct {
	resolved *jsonschema.Resolved
	bounds   map[float64]bool
}
