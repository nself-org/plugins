package stats

// Purpose: TestStatsGolden (P7-SCHED-20): the fixture store gives the golden contract:ci.stats documents for
// `--since 24h` over all projects and for `--project acme/a`; a store without cache, ledger or burst rows
// reports those blocks as null with a reason and no figure as 0; a scoped caller loses the coordinator-wide blocks.
// Inputs:  buildFixture(t, 400) and testdata/*.golden.json (rewrite with UPDATE_GOLDEN=1).
// Outputs: pass or fail.
// Constraints: the store size and free disk depend on the machine, so the golden documents are compared with
//          those two figures removed.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func golden(t *testing.T, name string, d Doc) {
	t.Helper()
	d.Store = nil // machine dependent
	delete(d.Reasons, "store.free_bytes")
	got, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", name+".golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err = os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s differs from the golden document:\n%s", name, got)
	}
}

func TestStatsGolden(t *testing.T) {
	st := buildFixture(t, 400)
	ctx := context.Background()
	all, err := Compute(ctx, st, Options{Now: fixtureNow, Since: 24 * time.Hour, Global: true})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "all", all)
	if all.Attempts == nil || all.Attempts.Finished != 500 {
		t.Fatalf("finished attempts = %+v, want 500", all.Attempts)
	}
	a, err := Compute(ctx, st, Options{Now: fixtureNow, Since: 24 * time.Hour, Project: "acme/a", Global: false})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "project-a", a)
}

func TestStatsAbsentSourcesAreNullNotZero(t *testing.T) {
	st := buildFixture(t, 4)
	d, err := Compute(context.Background(), st, Options{Now: fixtureNow.Add(100 * 24 * time.Hour), Since: time.Hour, Global: true})
	if err != nil {
		t.Fatal(err)
	}
	if d.Cache != nil || d.Spend != nil || d.Attempts != nil || d.Provisioning != nil || d.Runners != nil {
		t.Fatalf("an empty window gave figures: %+v", d)
	}
	for _, b := range d.Queue { // the three queued attempts still count; their wait has no samples
		if b.WaitMs != nil || d.Reasons["queue.normal.wait_ms"] != ReasonNoSamples {
			t.Errorf("queue band %+v / reasons %v", b, d.Reasons)
		}
	}
	for _, k := range []string{"cache", "spend", "attempts", "runners", "provisioning_ms"} {
		if d.Reasons[k] == "" {
			t.Errorf("no reason for null %s", k)
		}
	}
	raw, _ := json.Marshal(d)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for _, k := range []string{"cache", "spend", "attempts", "provisioning_ms"} {
		if m[k] != nil {
			t.Errorf("%s = %v, want null", k, m[k])
		}
	}
}

func TestStatsScopedCallerSeesOnlyAllowedProjects(t *testing.T) {
	st := buildFixture(t, 40)
	d, err := Compute(context.Background(), st, Options{Now: fixtureNow, Since: 24 * time.Hour, Allow: func(p string) bool { return p == "acme/b" }})
	if err != nil {
		t.Fatal(err)
	}
	if d.Nodes != nil || d.Outbox != nil || d.Store != nil || d.Reasons["nodes"] != ReasonScoped {
		t.Errorf("a scoped caller saw coordinator-wide blocks: %+v", d)
	}
	if d.Leases == nil || d.Leases.Stale != 1 { // the stale lease belongs to acme/b
		t.Errorf("leases = %+v", d.Leases)
	}
	for _, r := range d.Runners {
		if r.Attempts == 0 {
			t.Error("runner without attempts")
		}
	}
	if d.Attempts == nil || d.Attempts.Finished != 25 { // 20 jobs of b plus 5 retries
		t.Errorf("attempts = %+v", d.Attempts)
	}
}

func TestPercentilesAreNearestRank(t *testing.T) {
	v := make([]int64, 100)
	for i := range v {
		v[i] = int64(i + 1)
	}
	if p := pct(v); p.P50 != 50 || p.P95 != 95 || p.N != 100 {
		t.Errorf("pct = %+v", p)
	}
	if p := pct([]int64{7}); p.P50 != 7 || p.P95 != 7 {
		t.Errorf("single = %+v", p)
	}
	if pct(nil) != nil {
		t.Error("empty must be nil")
	}
	if permille(1, 3) != 333 || permille(2, 3) != 667 {
		t.Error("permille rounding")
	}
}
