package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCapabilityFixtures(t *testing.T) {
	schema, err := CapabilitySchema()
	if err != nil {
		t.Fatal(err)
	}
	validPaths, err := filepath.Glob("testdata/capability/valid/*.json")
	if err != nil {
		t.Fatal(err)
	}
	invalidPaths, err := filepath.Glob("testdata/capability/invalid/*.json")
	if err != nil {
		t.Fatal(err)
	}
	paths := append(validPaths, invalidPaths...)
	if len(paths) != 9 {
		t.Fatalf("fixtures: %d", len(paths))
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			err = ValidateJSON(schema, body)
			invalid := strings.Contains(path, "/invalid/")
			if invalid && (err == nil || !strings.Contains(err.Error(), "/")) {
				t.Fatalf("expected JSON pointer error: %v", err)
			}
			if !invalid && err != nil {
				t.Fatal(err)
			}
			if !invalid {
				var c Capability
				if e := json.Unmarshal(body, &c); e != nil {
					t.Fatal(e)
				}
				if e := ValidateCapability(c); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}

func TestCapabilityUnknownFieldGoPath(t *testing.T) {
	body, err := os.ReadFile("testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	raw["future_trust_override"] = true
	body, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var c Capability
	if err := json.Unmarshal(body, &c); err == nil || !strings.Contains(err.Error(), "future_trust_override") {
		t.Fatalf("unknown field accepted: %v", err)
	}
}

func TestCapabilitySchemaCurrent(t *testing.T) {
	want, err := CapabilitySchema()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../schemas/runner-capability.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Fatal("runner capability schema drift; run go generate ./internal/model/...")
	}
}

// D-1332: the probe's proof of enforced network scopes is an optional fact; only a probe may state it.
func TestCapabilityNetworkEnforcedScopes(t *testing.T) {
	body, err := os.ReadFile("testdata/capability/valid/vps.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c Capability
	if err := json.Unmarshal(body, &c); err != nil {
		t.Fatal(err)
	}
	if c.Network != nil {
		t.Fatal("a fixture without a probe result must have no network section")
	}
	scopes := []NetworkScope{"none", "restricted"}
	c.Network = &CapabilityNetwork{EnforcedScopes: Fact[[]NetworkScope]{Value: &scopes, Source: "probe", ObservedAt: c.Verified.At, Confidence: "known"}}
	if err := ValidateCapability(c); err != nil {
		t.Fatalf("probe fact refused: %v", err)
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var back Capability
	if err := json.Unmarshal(raw, &back); err != nil || back.Network == nil || len(*back.Network.EnforcedScopes.Value) != 2 {
		t.Fatalf("round trip: %v %+v", err, back.Network)
	}
	for name, mutate := range map[string]func(*Capability){
		"self source":     func(c *Capability) { c.Network.EnforcedScopes.Source = "self" },
		"assigned source": func(c *Capability) { c.Network.EnforcedScopes.Source = "assigned" },
		"unknown scope":   func(c *Capability) { v := []NetworkScope{"everything"}; c.Network.EnforcedScopes.Value = &v },
	} {
		bad := c
		n := *c.Network
		bad.Network = &n
		mutate(&bad)
		if err := ValidateCapability(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
