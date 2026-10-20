//go:build scenarios

package scenarios

// Purpose: the hybrid world of scenario 6 (Epic acceptance 3): local, two LAN nodes and a hosted
// runner priced from the D4 preset fixture (internal/sched/testdata/presets/world.json).
// Inputs:  the fixture file, read through a relative path from the test directory.
// Outputs: sched.Runner values, per-runner estimates and a world builder.
// Constraints: the fixture is read, never copied, so a changed fixture changes the scenario; the
//          hosted runner is owned by the P7-HOST-01 fake provider (internal/providers/fake).

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/fake"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// s6Row is one runner of the fixture, in the fixture's own field names.
type s6Row struct {
	ID         string `json:"id"`
	Class      string `json:"class"`
	Location   string `json:"location"`
	Wait       int64  `json:"wait_ms"`
	Runtime    int64  `json:"runtime_ms"`
	Cold       int64  `json:"cold_start_ms"`
	Transfer   int64  `json:"transfer_ms"`
	Contention int64  `json:"contention_ms"`
	Cost       int64  `json:"cost_micro"`
	Scarcity   int64  `json:"scarcity_ms"`
}

// s6Fixture reads the fixture rows by id.
func s6Fixture(t testing.TB) map[string]s6Row {
	t.Helper()
	raw, err := os.ReadFile("../../internal/sched/testdata/presets/world.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Runners []s6Row `json:"runners"`
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	out := map[string]s6Row{}
	for _, r := range f.Runners {
		out[r.ID] = r
	}
	for _, id := range []string{"L", "N", "H"} {
		if _, ok := out[id]; !ok {
			t.Fatalf("fixture has no runner %s", id)
		}
	}
	return out
}

// s6Hybrid returns the four runners and the rows that price them: local L, LAN N and N2 (N2 is N
// with 5000 ms more wait) and hosted H. extraWait adds queue wait to the three non-hosted runners.
func s6Hybrid(t testing.TB, extraWait int64) ([]sched.Runner, map[string]s6Row) {
	t.Helper()
	fx := s6Fixture(t)
	rows := map[string]s6Row{"L": fx["L"], "N": fx["N"], "N2": fx["N"], "H": fx["H"]}
	n2 := rows["N2"]
	n2.ID, n2.Wait = "N2", n2.Wait+5000
	rows["N2"] = n2
	for _, id := range []string{"L", "N", "N2"} {
		r := rows[id]
		r.Wait += extraWait
		rows[id] = r
	}
	hostedProvider := fake.New(fake.Options{}).ID()
	var rs []sched.Runner
	for _, id := range []string{"L", "N", "N2", "H"} {
		row := rows[id]
		r := runner(id, row.Location, "linux/amd64", 1)
		r.Class = row.Class
		if row.Location == "hosted" {
			r.Provider = hostedProvider
			r.Capability.Identity.Provider = hostedProvider
			r.Decl.Hosted = true
			r.Capability.Identity.Labels = append(r.Capability.Identity.Labels, "nself.visibility=public")
		}
		rs = append(rs, r)
	}
	return rs, rows
}

// s6Estimates prices every runner for job key key as the fixture does.
func s6Estimates(rows map[string]s6Row, key string) map[string]map[string]sched.Sample {
	out := map[string]map[string]sched.Sample{}
	for id, r := range rows {
		out[id] = map[string]sched.Sample{key: {WaitMs: ptr(r.Wait), RuntimeMs: ptr(r.Runtime), ColdStartMs: ptr(r.Cold), TransferMs: ptr(r.Transfer),
			ContentionMs: ptr(r.Contention), CostMicro: ptr(r.Cost), ScarcityMs: ptr(r.Scarcity), ReliabilityMs: ptr(int64(0)), CacheMs: ptr(int64(0)), Confidence: "known"}}
	}
	return out
}

// s6World builds a placement world for preset over the runners and rows.
func s6World(preset string, rs []sched.Runner, rows map[string]s6Row, key string) sched.World {
	reg, err := trust.NewRegistry(trust.KeySpec{Key: "trust.privacy_zone", Owner: "trust", Kind: trust.TightenLower, Order: []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}, Default: "hosted-allowed"})
	if err != nil {
		panic(err)
	}
	eff, _, err := reg.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.privacy_zone": json.RawMessage(`"hosted-allowed"`)}})
	if err != nil { // a project grant lets the hosted runner take the job (D12)
		panic(err)
	}
	w := sched.World{Policy: sched.Policy{Preset: preset, Trust: eff}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}, Estimates: s6Estimates(rows, key)}
	for _, r := range rs {
		if r.Location == "hosted" {
			w.Hosted = append(w.Hosted, r)
		} else {
			w.Runners = append(w.Runners, r)
		}
	}
	return w
}

// s6Scores returns each alternative's score and whether it was eligible, from one explanation.
func s6Scores(x sched.Explanation) (scores map[string]int64, ids []string) {
	scores = map[string]int64{}
	for _, a := range x.Alternatives {
		ids = append(ids, a.RunnerID)
		if a.Score != nil {
			scores[a.RunnerID] = *a.Score
		}
	}
	return scores, ids
}
