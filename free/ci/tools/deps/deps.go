//go:build tools

package deps

import (
	_ "github.com/google/jsonschema-go/jsonschema"
	_ "github.com/nself-org/cli/sdk/go/v2/compat"
	_ "github.com/nself-org/cli/sdk/go/v2/output"
	_ "gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)
