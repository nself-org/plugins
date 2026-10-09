package model

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestTriggerFactsGolden proves byte-stable JSON and explicit null for unknown facts.
func TestTriggerFactsGolden(t *testing.T) {
	f := TriggerFacts{Schema: "ci.trigger-facts/v1", Source: "source-1", Kind: "runner_demand", ReceivedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Auth: "per-repo", Repo: "org/repo", RevisionSource: FactPayload, RunEvent: Fact[string]{Source: "api"}}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "trigger_facts", "runner_demand.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(b, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(append(b, '\n'), want) {
		t.Fatalf("golden mismatch\n got %s\nwant %s", b, want)
	}
	if !bytes.Contains(b, []byte(`"run_event":{"value":null,"source":"api"`)) {
		t.Fatal("unknown fact must encode null")
	}
	var out TriggerFacts
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(out)
	if !bytes.Equal(b, again) {
		t.Fatal("round trip changed bytes")
	}
	if err := out.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*TriggerFacts){func(x *TriggerFacts) { x.Kind = "alien" }, func(x *TriggerFacts) { x.Auth = "alien" }, func(x *TriggerFacts) { x.RunEvent.Source = "alien" }} {
		bad := out
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("unknown enum accepted")
		}
	}
}
