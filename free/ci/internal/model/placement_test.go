package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlacementContract(t *testing.T) {
	p := PlacementPlan{Schema: "ci.placement-plan/v1", Pipeline: PlacementPipelineRef{Name: "p", Preset: "balanced"}, FastPath: FastPath{Reason: "none"}, Jobs: []JobPlacement{{ID: "j", Status: "waiting", Reasons: []PlacementReasonDetail{{Code: string(CapacityBusy), Detail: "busy"}}, Alternatives: []Alternative{}}}, Totals: Totals{Waiting: 1, CostConfidence: "unknown"}}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"schema":"ci.placement-plan/v1"`, `"fast_path"`, `"alternatives":[]`, `"placement":null`, `"cost_confidence":"unknown"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("missing %s in %s", key, raw)
		}
	}
}
func TestPlacementReasonsClosed(t *testing.T) {
	for reason := range placementReasons {
		if !reason.Valid() {
			t.Fatalf("invalid listed reason %s", reason)
		}
	}
	if PlacementReason("arbitrary.reason").Valid() {
		t.Fatal("open reason list")
	}
	if !PlatformMismatch.Static() || CapacityBusy.Static() {
		t.Fatal("static classification")
	}
}
