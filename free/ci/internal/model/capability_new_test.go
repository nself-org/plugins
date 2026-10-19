package model

import (
	"encoding/json"
	"testing"
	"time"
)

// NewCapability is the one scaffold both node builders (lifecycle.newCapability, provision.RunnerCapability)
// start from: every fact unknown, observed facts from the probe source and self-reported ones from the self source.
func TestNewCapabilityIsAllUnknownScaffold(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	c := NewCapability("probe", "self", now)
	if c.Schema != "ci.runner-capability/v1" || c.Identity.Provider != "ssh" || !c.Lifecycle.Discovered {
		t.Fatalf("scaffold header %+v", c)
	}
	if c.Identity.Labels == nil || c.Identity.HostKeyFingerprints == nil || c.Tools.Toolchains == nil || c.Tools.Custom == nil || c.Lifecycle.AuthorizedProjects == nil {
		t.Fatal("a list or map is nil: it would marshal as null")
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	facts := 0
	var walk func(path string, v any)
	walk = func(path string, v any) {
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		if _, isFact := m["observed_at"]; isFact {
			facts++
			if m["confidence"] != "unknown" || m["value"] != nil {
				t.Errorf("%s is not an unknown fact: %v", path, m)
			}
			return
		}
		for k, sub := range m {
			walk(path+"."+k, sub)
		}
	}
	walk("", doc)
	if facts < 40 {
		t.Fatalf("only %d facts found in the scaffold", facts)
	}
	for _, tc := range []struct{ name, got, want string }{
		{"platform.kernel", c.Platform.Kernel.Source, "probe"},
		{"tools.docker", c.Tools.Docker.Source, "probe"},
		{"availability.battery", c.Availability.BatteryPct.Source, "probe"},
		{"identity.machine_id", c.Identity.MachineID.Source, "self"},
		{"availability.slots", c.Availability.Slots.Source, "self"},
		{"location.kind", c.Location.Kind.Source, "self"},
		{"economics.class", c.Economics.Class.Source, "self"},
		{"identity.ownership", c.Identity.Ownership.Source, "assigned"},
		{"resources.reserved", c.Resources.Reserved.CPU.Source, "assigned"},
		{"trust.accepts", c.Trust.Accepts.Source, "assigned"},
		{"trust.privacy_zone", c.Trust.PrivacyZone.Source, "assigned"},
		{"availability.persistence", c.Availability.Persistence.Source, "assigned"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s source %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if c.Verified.By != "self" || !c.Verified.At.Equal(now) || c.Protocol.Min != 1 || len(c.Protocol.Versions) != 1 {
		t.Fatalf("verified/protocol %+v %+v", c.Verified, c.Protocol)
	}
}
