package history

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var epoch = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// fixture holds a store whose clock the test moves and a state directory.
type fixture struct {
	t    *testing.T
	st   *store.Store
	home string
	now  time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, home: t.TempDir(), now: epoch}
	st, err := store.Open(filepath.Join(f.home, "state.db"), store.Options{Clock: func() time.Time { return f.now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f.st = st
	return f
}

// add records a pipeline created `age` ago with a log of about `bytes` bytes.
func (f *fixture) add(id, status string, age time.Duration, bytes int) {
	f.t.Helper()
	ctx := context.Background()
	f.now = epoch.Add(-age)
	row := store.PipelineRow{ID: id, Revision: "rev", Project: "p", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "d", InputDigest: "i", SelectionMode: "full", Status: "running"}
	if err := f.st.CreatePipeline(ctx, row); err != nil {
		f.t.Fatal(err)
	}
	if err := f.st.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: id, Name: "job", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		f.t.Fatal(err)
	}
	if err := f.st.CreateAttempt(ctx, store.AttemptRow{ID: "a" + id, JobID: "j" + id, N: 1}, ""); err != nil {
		f.t.Fatal(err)
	}
	if status != "running" {
		f.now = f.now.Add(time.Minute)
		if err := f.st.FinishPipeline(ctx, id, status); err != nil {
			f.t.Fatal(err)
		}
	}
	if bytes > 0 {
		f.log(id, "a"+id, bytes)
	}
}

func (f *fixture) log(run, attempt string, bytes int) {
	f.t.Helper()
	w, err := logs.Open(logs.AttemptDir(f.home, run, attempt), logs.Options{})
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = w.Append(logs.Line{TS: "t", Stream: "out", Seq: 1, Text: strings.Repeat("x", bytes)}); err != nil {
		f.t.Fatal(err)
	}
	_ = w.Close()
}

func (f *fixture) ids() []string {
	f.t.Helper()
	ages, err := f.st.PipelineAges(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	var out []string
	for _, a := range ages {
		out = append(out, a.ID)
	}
	return out
}

func (f *fixture) logRuns() []string {
	f.t.Helper()
	infos, err := logs.ListRuns(f.home)
	if err != nil {
		f.t.Fatal(err)
	}
	var out []string
	for _, i := range infos {
		out = append(out, i.Run)
	}
	sort.Strings(out) // ListRuns orders by file time, which the fixture does not control
	return out
}

func (f *fixture) prune(pol Policy, keep ...string) PruneResult {
	f.t.Helper()
	res, err := Prune(context.Background(), f.st, PruneParams{Home: f.home, Policy: pol, Keep: keep, Now: epoch})
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

func same(t *testing.T, label string, got, want []string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

const day = 24 * time.Hour

func TestPruneByCountAndAgeKeepsTheRunningRun(t *testing.T) {
	f := newFixture(t)
	f.add("old-running", "running", 100*day, 10)
	f.add("p1", "passed", 40*day, 10)
	f.add("p2", "failed", 20*day, 10)
	f.add("p3", "passed", 10*day, 10)
	f.add("p4", "passed", 5*day, 10)
	f.add("p5", "passed", day, 10)
	res := f.prune(Policy{KeepRuns: 3, MaxAgeDays: 30, MaxLogBytes: 1 << 30})
	same(t, "removed", res.Runs, []string{"p1", "p2"})
	same(t, "store rows (newest first)", f.ids(), []string{"p5", "p4", "p3", "old-running"})
	same(t, "log dirs after prune", f.logRuns(), []string{"old-running", "p3", "p4", "p5"})
	if res.FreedBytes == 0 {
		t.Error("no freed bytes reported")
	}
}

func TestPruneAgeAloneAndKeepList(t *testing.T) {
	f := newFixture(t)
	f.add("a", "passed", 45*day, 10)
	f.add("b", "passed", 44*day, 10)
	f.add("c", "passed", day, 10)
	res := f.prune(Policy{KeepRuns: 200, MaxAgeDays: 30, MaxLogBytes: 1 << 30}, "a")
	same(t, "removed", res.Runs, []string{"b"})
	same(t, "rows", f.ids(), []string{"c", "a"})
}

func TestPruneSizeCapTakesTheOldestFirst(t *testing.T) {
	f := newFixture(t)
	f.add("r1", "passed", 5*day, 3000)
	f.add("r2", "passed", 4*day, 3000)
	f.add("r3", "passed", 3*day, 3000)
	f.add("live", "running", 2*day, 3000)
	res := f.prune(Policy{KeepRuns: 200, MaxAgeDays: 30, MaxLogBytes: 7000})
	same(t, "removed", res.Runs, []string{"r1", "r2"})
	same(t, "rows", f.ids(), []string{"live", "r3"})
	same(t, "log dirs", f.logRuns(), []string{"live", "r3"})
}

func TestPruneDryRunChangesNothing(t *testing.T) {
	f := newFixture(t)
	f.add("old", "passed", 90*day, 50)
	f.add("new", "passed", day, 50)
	res, err := Prune(context.Background(), f.st, PruneParams{Home: f.home, Policy: Default(), DryRun: true, Now: epoch})
	if err != nil {
		t.Fatal(err)
	}
	same(t, "would remove", res.Runs, []string{"old"})
	same(t, "rows", f.ids(), []string{"new", "old"})
	same(t, "log dirs", f.logRuns(), []string{"new", "old"})
}

// A crash between the store delete and the file delete leaves a log directory
// with no run: the next prune sweeps it once it is older than the grace.
func TestPruneSweepsOrphanLogDirectories(t *testing.T) {
	f := newFixture(t)
	f.add("keep", "passed", day, 10)
	f.log("orphan-old", "x", 10)
	f.log("orphan-new", "x", 10)
	old := epoch.Add(-3 * time.Hour)
	for _, p := range []string{logs.RunDir(f.home, "orphan-old"), logs.AttemptDir(f.home, "orphan-old", "x"), filepath.Join(logs.AttemptDir(f.home, "orphan-old", "x"), "active.jsonl")} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	fresh := epoch.Add(-time.Minute) // within the grace: a run being created
	for _, p := range []string{logs.RunDir(f.home, "orphan-new"), logs.AttemptDir(f.home, "orphan-new", "x"), filepath.Join(logs.AttemptDir(f.home, "orphan-new", "x"), "active.jsonl")} {
		_ = os.Chtimes(p, fresh, fresh)
	}
	res := f.prune(Default())
	same(t, "orphans", res.Orphans, []string{"orphan-old"})
	same(t, "log dirs", f.logRuns(), []string{"keep", "orphan-new"})
	same(t, "rows", f.ids(), []string{"keep"})
}

func TestPolicyFileOverridesDefaultsAndBadFileKeepsThem(t *testing.T) {
	home := t.TempDir()
	if p, err := LoadPolicy(home); err != nil || p != Default() {
		t.Fatalf("no file: %+v %v", p, err)
	}
	if err := os.WriteFile(ConfigPath(home), []byte(`{"keep_runs": 5, "max_age_days": 0, "max_log_bytes": 1000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPolicy(home)
	if err != nil || p.KeepRuns != 5 || p.MaxAgeDays != DefaultMaxAgeDays || p.MaxLogBytes != 1000 {
		t.Fatalf("policy %+v %v", p, err)
	}
	_ = os.WriteFile(ConfigPath(home), []byte(`{nope`), 0o600)
	if p, err = LoadPolicy(home); err == nil || p != Default() {
		t.Fatalf("a malformed file must report an error and keep the defaults: %+v %v", p, err)
	}
}

func TestDefaultsAreTheDocumentedOnes(t *testing.T) {
	d := Default()
	if d.KeepRuns != 200 || d.MaxAgeDays != 30 || d.MaxLogBytes != 2<<30 {
		t.Fatalf("defaults %+v", d)
	}
}

// The first attempt of a run triggers the prune: old runs beyond the user's
// retention.json go, the new (running) run stays, and logs follow the rows.
func TestRunStartPrunesAccordingToTheRetentionFile(t *testing.T) {
	f := newFixture(t)
	f.add("p1", "passed", 3*day, 20)
	f.add("p2", "passed", 2*day, 20)
	f.add("p3", "passed", day, 20)
	if err := os.WriteFile(ConfigPath(f.home), []byte(`{"keep_runs": 2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = f.st.Close()
	res := runOnce(t, f.home, "echo hi")
	st, err := store.Open(filepath.Join(f.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ages, _ := st.PipelineAges(context.Background())
	var ids []string
	for _, a := range ages {
		ids = append(ids, a.ID)
	}
	same(t, "rows after the run", ids, []string{res.Summary.RunID, "p3"})
	infos, _ := logs.ListRuns(f.home)
	var dirs []string
	for _, i := range infos {
		dirs = append(dirs, i.Run)
	}
	sort.Strings(dirs)
	want := []string{"p3", res.Summary.RunID}
	sort.Strings(want)
	same(t, "log dirs after the run", dirs, want)
}

// A huge day count means "never by age": it must not overflow into a negative
// age that deletes everything.
func TestHugeMaxAgeDaysPrunesNothingByAge(t *testing.T) {
	for _, days := range []int{106751, 106752, 999999, 1 << 40} {
		if p := (Policy{MaxAgeDays: days}); p.MaxAge() <= 0 {
			t.Fatalf("MaxAge(%d days) = %v", days, p.MaxAge())
		}
	}
	f := newFixture(t)
	f.add("ancient", "passed", 400*day, 10)
	f.add("recent", "passed", day, 10)
	res := f.prune(Policy{KeepRuns: 200, MaxAgeDays: 999999, MaxLogBytes: 1 << 30})
	if len(res.Runs) != 0 {
		t.Fatalf("removed %v", res.Runs)
	}
	same(t, "rows", f.ids(), []string{"recent", "ancient"})
}

// Orphan log directories are swept anyway, so their bytes must not push real
// runs out through the size cap.
func TestOrphanBytesDoNotCountAgainstTheSizeCap(t *testing.T) {
	f := newFixture(t)
	f.add("real", "passed", day, 2000)
	f.log("orphan", "x", 20000)
	old := epoch.Add(-3 * time.Hour)
	for _, p := range []string{logs.RunDir(f.home, "orphan"), logs.AttemptDir(f.home, "orphan", "x"), filepath.Join(logs.AttemptDir(f.home, "orphan", "x"), "active.jsonl")} {
		_ = os.Chtimes(p, old, old)
	}
	res := f.prune(Policy{KeepRuns: 200, MaxAgeDays: 30, MaxLogBytes: 5000})
	same(t, "removed runs", res.Runs, nil)
	same(t, "orphans", res.Orphans, []string{"orphan"})
	same(t, "rows", f.ids(), []string{"real"})
}

func TestPruneResultListsAreNeverNullAndRunStartPruneIsBounded(t *testing.T) {
	f := newFixture(t)
	f.add("keep", "passed", day, 10)
	res := f.prune(Default())
	if res.Runs == nil || res.Orphans == nil {
		t.Fatalf("nil lists would print JSON null: %+v", res)
	}
	for i := 0; i < 8; i++ {
		f.add(fmt.Sprintf("old%d", i), "passed", time.Duration(60+i)*day, 10)
	}
	res, err := Prune(context.Background(), f.st, PruneParams{Home: f.home, Policy: Default(), Now: epoch, MaxRuns: 3})
	if err != nil {
		t.Fatal(err)
	}
	same(t, "removed (oldest first, bounded)", res.Runs, []string{"old7", "old6", "old5"})
	if len(f.ids()) != 6 {
		t.Fatalf("rows left: %v", f.ids())
	}
}
