package trust

import "encoding/json"

func DefaultPolicy(visibility string) Scope {
	zone := "private-infrastructure"
	if visibility == "public" {
		zone = "hosted-allowed"
	}
	values := map[string]json.RawMessage{
		"trust.isolation_min":       json.RawMessage(`"sandboxed-container"`),
		"trust.network":             json.RawMessage(`"restricted"`),
		"trust.privacy_zone":        json.RawMessage(`"` + zone + `"`),
		"trust.secrets":             json.RawMessage(`[]`),
		"trust.approval.first_time": json.RawMessage(`true`),
	}
	return Scope{Kind: SourceDefault, Values: values}
}
