package trust

import (
	"encoding/json"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// DefaultPolicy treats an omitted or unknown class as untrusted.
func DefaultPolicy(visibility string, classes ...model.TrustClass) Scope {
	zone := "private-infrastructure"
	if visibility == "public" {
		zone = "hosted-allowed"
	}
	values := map[string]json.RawMessage{
		"trust.privacy_zone": json.RawMessage(`"` + zone + `"`),
	}
	if len(classes) == 0 || DecodeTrust(string(classes[0])) == "untrusted" {
		values["trust.isolation_min"] = json.RawMessage(`"sandboxed-container"`)
		values["trust.network"] = json.RawMessage(`"restricted"`)
		values["trust.secrets"] = json.RawMessage(`[]`)
		if visibility == "public" {
			values["trust.approval.first_time"] = json.RawMessage(`true`)
		}
	}
	return Scope{Kind: SourceDefault, Values: values, publicVisibility: visibility == "public"}
}
