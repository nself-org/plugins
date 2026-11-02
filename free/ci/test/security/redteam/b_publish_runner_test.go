//go:build security

package redteam

// Purpose: the structural half of the publication attack for the runner records. Every string leaf
// under Evidence.Runner, Evidence.Attempts and Evidence.Errors is found by reflection (no field list),
// filled with private data in turn, and must not survive attest.Publish. A string field added to those
// records later is attacked automatically; it fails here until the publish path scrubs it.
// Inputs:  a reflectively populated model.Evidence. Outputs: pass or fail.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestB_Publish_RunnerRecordsAreScrubbedStructurally(t *testing.T) {
	var base model.Evidence
	fill(reflect.ValueOf(&base).Elem(), 0)
	var probe []func(string)
	var names []string
	leaves(reflect.ValueOf(&base).Elem(), "Evidence", &probe, &names)
	covered := 0
	for i, name := range names {
		if !strings.HasPrefix(name, "Evidence.Runner.") && !strings.HasPrefix(name, "Evidence.Attempts[") && !strings.HasPrefix(name, "Evidence.Errors[") {
			continue
		}
		covered++
		var ev model.Evidence
		fill(reflect.ValueOf(&ev).Elem(), 0)
		var set []func(string)
		var nm []string
		leaves(reflect.ValueOf(&ev).Elem(), "Evidence", &set, &nm)
		set[i](hostile(coreMarkers))
		out, err := json.Marshal(attest.Publish(ev, attest.Public))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range coreMarkers {
			if strings.Contains(strings.ToLower(string(out)), strings.ToLower(strings.ReplaceAll(m, `\`, `\\`))) || strings.Contains(string(out), m) {
				t.Errorf("published document keeps %s from %s", m, name)
			}
		}
	}
	if covered < 10 { // Runner (4) + Attempts runner (8) + Errors (3) at the time of writing
		t.Fatalf("fixture error: only %d runner-record string leaves found", covered)
	}
}
