package lifecycle

import (
	"encoding/json"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// marshalCapability encodes the capability document stored for a node.
func marshalCapability(c model.Capability) ([]byte, error) { return json.Marshal(c) }
