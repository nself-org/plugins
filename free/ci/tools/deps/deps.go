//go:build tools

package deps

import (
	_ "github.com/google/jsonschema-go/jsonschema"
	_ "github.com/hashicorp/mdns"
	_ "github.com/nself-org/cli/sdk/go/v2/compat"
	_ "github.com/nself-org/cli/sdk/go/v2/output"
	_ "github.com/nself-org/cli/sdk/go/v2/remote"
	_ "github.com/nself-org/cli/sdk/go/v2/signing"
	_ "github.com/nself-org/cli/sdk/go/v2/simharness"
	_ "gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)
