package model

import (
	"encoding/json"
	"testing"
	"time"
)

// confidences collects every "confidence" string under v.
func confidences(v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if s, ok := e.(string); ok && k == "confidence" {
				*out = append(*out, s)
				continue
			}
			confidences(e, out)
		}
	case []any:
		for _, e := range x {
			confidences(e, out)
		}
	}
}

func TestNewCapabilityIsAllUnknownAndValidWhenIdentified(t *testing.T) {
	now := time.Unix(1e9, 0).UTC()
	c := NewCapability("probe", "self", now)
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var got []string
	confidences(doc, &got)
	if len(got) < 30 {
		t.Fatalf("only %d facts found, the draft lost facts", len(got))
	}
	for _, g := range got {
		if g != "unknown" {
			t.Fatalf("a draft fact has confidence %q, want unknown", g)
		}
	}
	if err := ValidateCapability(c); err == nil {
		t.Fatal("an unidentified draft must not validate")
	}
	c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = "n1", "n1", "ssh", "ssh"
	c.Verified = CapabilityVerified{By: "self", At: now}
	c.Protocol = CapabilityProtocol{Versions: []int{1}, Min: 1}
	c.Trust.Accepts = KnownFact([]TrustClass{}, "assigned", now)
	if err := ValidateCapability(c); err != nil {
		t.Fatalf("identified draft: %v", err)
	}
}

func TestNewCapabilitySourcesFollowTheArguments(t *testing.T) {
	now := time.Unix(1e9, 0).UTC()
	c := NewCapability("provider", "self", now)
	if c.Resources.CPU.Source != "provider" || c.Identity.MachineID.Source != "self" || c.Resources.Reserved.CPU.Source != "assigned" {
		t.Fatalf("sources: cpu=%q machine=%q reserved=%q", c.Resources.CPU.Source, c.Identity.MachineID.Source, c.Resources.Reserved.CPU.Source)
	}
}
