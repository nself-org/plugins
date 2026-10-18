package policy

// Purpose: the trust.* keys (Epic D3, D27). Defaults are the loosest value, so
// every scope can only tighten; trust.DefaultPolicy then sets the starting
// point by repository visibility and source class.
// Inputs: none. Outputs: the keys, registered from init().
// Constraints: the Decode functions fail closed (an unknown order value
// becomes the most restrictive one, like the trust package's own decode);
// none of these keys is a Spend key.

import (
	"bytes"
	"encoding/json"
	"fmt"
)

var (
	isolationOrder = []string{"process", "container", "sandboxed-container", "vm", "ephemeral-vm"}
	networkOrder   = []string{"none", "restricted", "internet", "lan", "privileged"}
	privacyOrder   = []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}
	trustOrder     = []string{"untrusted", "collaborator", "internal", "owner"}
	secretClasses  = []string{"project", "environment", "team", "release", "deploy"}
)

func init() {
	for _, spec := range trustKeys() {
		if err := RegisterKey(spec); err != nil {
			panic(err)
		}
	}
}

func trustKeys() []KeySpec {
	return []KeySpec{
		{Key: "trust.isolation_min", Owner: "trust", Kind: TightenHigher, Order: isolationOrder, Default: "process", Decode: orderDecoder(TightenHigher, isolationOrder)},
		{Key: "trust.network", Owner: "trust", Kind: TightenLower, Order: networkOrder, Default: "privileged", Decode: orderDecoder(TightenLower, networkOrder)},
		{Key: "trust.privacy_zone", Owner: "trust", Kind: TightenLower, Order: privacyOrder, Default: "hosted-allowed", Decode: orderDecoder(TightenLower, privacyOrder)},
		{Key: "trust.providers", Owner: "trust", Kind: TightenSubset, Decode: listDecode},
		{Key: "trust.secrets", Owner: "trust", Kind: TightenSubset, Default: secretClasses, Decode: listDecode},
		{Key: "trust.accepts", Owner: "trust", Kind: TightenSubset, Default: trustOrder, Decode: listDecode},
		{Key: "trust.approval.first_time", Owner: "trust", Kind: TightenOnlyOn, Default: false, Decode: boolDecode},
		{Key: "trust.self_reported_gates", Owner: "trust", Kind: TightenSubset, Decode: listDecode},
		// D27: caps the source class of a machine-invoked run. Default owner = no cap.
		{Key: "trust.machine_invoked_cap", Owner: "trust", Kind: TightenLower, Order: trustOrder, Default: "owner", Decode: orderDecoder(TightenLower, trustOrder)},
	}
}

func decodeOne(raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	if d.More() {
		return nil, fmt.Errorf("trailing JSON value")
	}
	if v == nil {
		return nil, fmt.Errorf("null value")
	}
	return v, nil
}

func orderDecoder(kind Tightening, order []string) func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeOne(raw)
		if err != nil {
			return nil, err
		}
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected a string")
		}
		for _, o := range order {
			if o == s {
				return s, nil
			}
		}
		if kind == TightenLower {
			return order[0], nil
		}
		return order[len(order)-1], nil
	}
}

func listDecode(raw json.RawMessage) (any, error) {
	v, err := decodeOne(raw)
	if err != nil {
		return nil, err
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a string list")
	}
	for _, item := range list {
		if _, ok := item.(string); !ok {
			return nil, fmt.Errorf("expected a string list")
		}
	}
	return list, nil
}

func boolDecode(raw json.RawMessage) (any, error) {
	v, err := decodeOne(raw)
	if err != nil {
		return nil, err
	}
	if _, ok := v.(bool); !ok {
		return nil, fmt.Errorf("expected a boolean")
	}
	return v, nil
}
