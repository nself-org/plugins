package model

import (
	"testing"
	"time"
)

func TestNewCapabilityUnknownFacts(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	for _, source := range []string{"probe", "provider"} {
		t.Run(source, func(t *testing.T) {
			c := NewCapability(source, source, now)
			facts := []struct {
				value              any
				source, confidence string
				at                 time.Time
			}{
				{c.Resources.CPU.Value, c.Resources.CPU.Source, c.Resources.CPU.Confidence, c.Resources.CPU.ObservedAt},
				{c.Tools.Docker.Value, c.Tools.Docker.Source, c.Tools.Docker.Confidence, c.Tools.Docker.ObservedAt},
				{c.Location.Kind.Value, c.Location.Kind.Source, c.Location.Kind.Confidence, c.Location.Kind.ObservedAt},
				{c.Economics.Class.Value, c.Economics.Class.Source, c.Economics.Class.Confidence, c.Economics.Class.ObservedAt},
			}
			for i, f := range facts {
				if f.source != source || f.confidence != "unknown" || !f.at.Equal(now) {
					t.Errorf("fact %d: %+v", i, f)
				}
			}
			if c.Resources.CPU.Value != nil || c.Tools.Docker.Value != nil || c.Location.Kind.Value != nil || c.Economics.Class.Value != nil {
				t.Fatal("unknown facts carry values")
			}
			if c.Schema != "ci.runner-capability/v1" || c.Resources.Reserved.CPU.Source != "assigned" || c.Resources.Reserved.MemMB.Source != "assigned" {
				t.Fatal("schema or reservation source")
			}
			if c.Tools.Toolchains == nil || c.Tools.Custom == nil {
				t.Fatal("tool maps must be initialized")
			}
			c.Tools.Custom["one"] = Fact[string]{}
			if len(NewCapability(source, source, now).Tools.Custom) != 0 {
				t.Fatal("constructors share mutable maps")
			}
		})
	}
}
