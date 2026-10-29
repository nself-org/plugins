package main

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func TestPresetTableCoversEveryPreset(t *testing.T) {
	got := presetTable()
	for name := range sched.Presets() {
		if !strings.Contains(got, "| `"+name+"` |") {
			t.Errorf("preset %s missing", name)
		}
	}
	var p sched.Preset
	if len(presetComponents) != len(p.Weights) || len(presetClasses) != len(p.ClassBiasMs) {
		t.Fatal("column names out of step with sched.Preset")
	}
}

func TestPlacementReasonTable(t *testing.T) {
	got := placementReasonTable([]ReasonCode{
		{Value: "paid.disabled", Type: "PlacementReason"},
		{Value: "capacity.busy", Type: "PlacementReason"},
		{Value: "x", Type: "Reason"},
	})
	if !strings.Contains(got, "| `paid.disabled` | no (static) |") || !strings.Contains(got, "| `capacity.busy` | yes |") {
		t.Errorf("rows wrong:\n%s", got)
	}
	if strings.Contains(got, "`x`") {
		t.Error("non-placement reason leaked")
	}
}
