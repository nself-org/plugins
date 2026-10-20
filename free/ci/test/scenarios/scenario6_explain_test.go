//go:build scenarios

package scenarios

// Purpose: Epic acceptance 3 through `ci explain`'s own code. The hybrid world's placement result is
// rendered by planview.Build (the ci.placement-plan/v1 document) and planview.ExplainLines (the human
// lines): every runner is an alternative, the chosen one is the D4 fixture's first, and the ineligible
// one carries its reason code, so a change in planview that drops or blanks an alternative turns this red.
// Inputs:  the hybrid fixture world of s6_rig_test.go (local, two LAN fakes, hosted fake), one offline LAN node.
// Outputs: assertions only.
// Constraints: deterministic; no clock; the world digest is a fixed string.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func TestScenario6ExplainListsEveryAlternative(t *testing.T) {
	for preset, want := range s6Expected {
		rs, rows := s6Hybrid(t, 0)
		off := runner("N3", "lan", "linux/amd64", 1)
		off.Capability.Availability.State.Value = ptr("offline")
		rs = append(rs, off)
		rows["N3"] = rows["N"]
		w := s6World(preset, rs, rows, "j")
		jobs := []sched.Job{job("j", "linux/amd64")}
		res := sched.Place(w, jobs)
		doc := planview.Build(planview.Input{Name: "hybrid", Revision: strings.Repeat("a", 40), Trigger: "local", PolicyDigest: "p" + strings.Repeat("0", 63),
			WorldDigest: "w" + strings.Repeat("0", 63), World: w, Jobs: jobs, Result: res})
		if len(doc.Jobs) != 1 || doc.Jobs[0].Placement == nil || doc.Pipeline.Preset != preset {
			t.Fatalf("%s: plan document %+v", preset, doc)
		}
		j := doc.Jobs[0]
		first := strings.Fields(want.order)[0]
		if j.Placement.RunnerID != first {
			t.Fatalf("%s: chosen %s, the fixture's first choice is %s", preset, j.Placement.RunnerID, first)
		}
		if len(j.Alternatives) != 5 {
			t.Fatalf("%s: %d alternatives in the document, want all 5 runners", preset, len(j.Alternatives))
		}
		seen := map[string]model.Alternative{}
		for _, a := range j.Alternatives {
			seen[a.RunnerID] = a
		}
		for _, id := range []string{"H", "L", "N", "N2", "N3"} {
			if _, ok := seen[id]; !ok {
				t.Fatalf("%s: runner %s is missing from the explain document", preset, id)
			}
		}
		if n3 := seen["N3"]; n3.Eligible || !hasCode(n3.Reasons, string(model.NodeOffline)) {
			t.Fatalf("%s: the offline node must be listed with node.offline: %+v", preset, n3)
		}
		if len(j.Placement.Components) == 0 {
			t.Fatalf("%s: the chosen runner has no score components", preset)
		}
		lines := strings.Join(planview.ExplainLines(doc, planview.SourcePlan), "\n")
		for _, id := range []string{"H", "L", "N", "N2", "N3"} {
			if !strings.Contains(lines, "alt     "+id+"  ") {
				t.Fatalf("%s: the explain text omits runner %s:\n%s", preset, id, lines)
			}
		}
		if !strings.Contains(lines, "chosen  "+first) || !strings.Contains(lines, "node.offline") {
			t.Fatalf("%s: the explain text lacks the chosen runner or the offline reason:\n%s", preset, lines)
		}
		raw, err := json.Marshal(doc)
		if err != nil || strings.Contains(string(raw), `"alternatives":null`) || strings.Contains(string(raw), `"jobs":null`) {
			t.Fatalf("%s: the document has a null array (%v)", preset, err)
		}
	}
}
