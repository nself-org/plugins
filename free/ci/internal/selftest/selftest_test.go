package selftest

// Purpose: P7-AUTO-20 acceptance for the pure half of the selftest: planted
// faults flip the matching check, the alert goes out once per failing set per
// UTC day, and a tidy is report-only unless applied.
// Inputs: hand-built Facts, a fake notifier, fake tidy steps, a temp state file.
// Outputs: pass or fail. Constraints: no clock reads, no real disk or network.

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/notify"
)

var day1 = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

// healthy is a host with nothing wrong: every selftest check passes.
func healthy() Facts {
	return Facts{
		Now:      day1,
		Tools:    []Tool{{Name: "go", Found: true, Required: true}, {Name: "gitleaks", Found: true}},
		Provider: Provider{LoggedIn: true, Reachable: true},
		Repo:     Probe{OK: true},
		Disk:     Disk{FreeMB: 100000},
		Sinks:    Sinks{List: []Sink{{Name: "ntfy", Delivered: 3}}},
		Watcher:  Watcher{Present: true, Age: time.Minute, Interval: time.Minute},
		Claims:   Claims{Known: true, RunLimit: time.Hour, List: []Claim{{Ref: "o/r@aaa", Age: time.Minute}}},
		Infra:    Infra{Total: 10},
	}
}

func statusOf(checks []doctor.Check) map[string]doctor.Status {
	out := map[string]doctor.Status{}
	for _, c := range checks {
		out[c.ID] = c.Status
	}
	return out
}

func TestSelftestChecksFlip(t *testing.T) {
	l := DefaultLimits()
	base := Evaluate(healthy(), l)
	if len(base) != 9 {
		t.Fatalf("want nine selftest checks, got %d", len(base))
	}
	for _, c := range base {
		if c.Status != doctor.Pass {
			t.Fatalf("a healthy host must pass every check, %s is %s: %s", c.ID, c.Status, c.Message)
		}
	}
	plants := []struct {
		id    string
		plant func(*Facts)
	}{
		{IDClaims, func(f *Facts) {
			f.Claims.List = append(f.Claims.List, Claim{Ref: "o/r@bbb", Age: time.Hour + 11*time.Minute})
		}},
		{IDOrphans, func(f *Facts) { f.Orphans.List = []string{"job-root-123"} }},
		{IDInfra, func(f *Facts) { f.Infra.Count = l.InfraFail }},
		{IDWatcher, func(f *Facts) { f.Watcher.Age = 4 * time.Minute }},
		{IDDisk, func(f *Facts) { f.Disk.FreeMB = l.MinFreeMB - 1 }},
		{IDTools, func(f *Facts) { f.Tools[0].Found = false }},
	}
	for _, p := range plants {
		t.Run(p.id, func(t *testing.T) {
			f := healthy()
			p.plant(&f)
			got := statusOf(Evaluate(f, l))
			for id, st := range got {
				want := doctor.Pass
				if id == p.id {
					want = doctor.Fail
				}
				if st != want {
					t.Fatalf("planting %s: %s is %s, want %s", p.id, id, st, want)
				}
			}
		})
	}
	// A fact that could not be read warns with the cause; it is never a pass.
	f := healthy()
	f.Orphans.Err = "permission denied"
	if got := statusOf(Evaluate(f, l))[IDOrphans]; got != doctor.Warn {
		t.Fatalf("an unreadable fact must warn, got %s", got)
	}
}

// fakeSink records what it was sent; fail makes it refuse.
type fakeSink struct {
	got  []notify.Event
	fail bool
}

func (s *fakeSink) Name() string { return "fake" }
func (s *fakeSink) Notify(_ context.Context, ev notify.Event) error {
	if s.fail {
		return errors.New("refused")
	}
	s.got = append(s.got, ev)
	return nil
}

func failing(ids ...string) []doctor.Check {
	out := []doctor.Check{{ID: IDTools, Name: "gate tools", Status: doctor.Pass}}
	for _, id := range ids {
		out = append(out, doctor.Check{ID: id, Name: "check " + id, Status: doctor.Fail})
	}
	return out
}

func TestSelftestAlertOncePerDay(t *testing.T) {
	ctx := context.Background()
	sink := &fakeSink{}
	now := day1
	a := Alerter{Sinks: []notify.Notifier{sink}, StatePath: filepath.Join(t.TempDir(), "alert.json"), Host: "mini1", Now: func() time.Time { return now }}
	steps := []struct {
		name   string
		at     time.Time
		checks []doctor.Check
		sent   bool
	}{
		{"first failing set is sent", day1, failing(IDClaims), true},
		{"the same set later that day is not", day1.Add(10 * time.Hour), failing(IDClaims), false},
		{"one more failing check is a new set", day1.Add(11 * time.Hour), failing(IDClaims, IDOrphans), true},
		{"a clean report sends nothing", day1.Add(12 * time.Hour), failing(), false},
		{"the next UTC day sends the set again", day1.Add(24 * time.Hour), failing(IDClaims), true},
	}
	want := 0
	for _, s := range steps {
		now = s.at
		res, err := a.Alert(ctx, s.checks)
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if res.Sent != s.sent {
			t.Fatalf("%s: sent=%v (%+v)", s.name, res.Sent, res)
		}
		if s.sent {
			want++
		}
		if len(sink.got) != want {
			t.Fatalf("%s: %d message(s) sent, want %d", s.name, len(sink.got), want)
		}
	}
	if msg := sink.got[1].Message; !strings.Contains(msg, IDClaims) || !strings.Contains(msg, IDOrphans) || !strings.Contains(msg, "mini1") {
		t.Fatalf("the message must name the host and every failing id: %q", msg)
	}
	// A set no sink took is not recorded, so the next run tries again.
	refusing := &fakeSink{fail: true}
	b := Alerter{Sinks: []notify.Notifier{refusing}, StatePath: filepath.Join(t.TempDir(), "alert.json"), Now: func() time.Time { return day1 }}
	if res, _ := b.Alert(ctx, failing(IDInfra)); res.Sent {
		t.Fatal("a refused message must not count as sent")
	}
	b.Sinks = []notify.Notifier{sink}
	if res, err := b.Alert(ctx, failing(IDInfra)); err != nil || !res.Sent {
		t.Fatalf("a set no sink took must be tried again: %+v %v", res, err)
	}
}

// tidyRig is a disk whose free space grows by what the steps remove.
type tidyRig struct {
	free    int64
	applied map[string][]Item
}

func (r *tidyRig) step(name string, items ...Item) Step {
	return Step{Name: name,
		Plan: func(context.Context) ([]Item, error) { return items, nil },
		Apply: func(_ context.Context, got []Item) error {
			r.applied[name] = got
			for _, it := range got {
				r.free += it.Bytes >> 20
			}
			return nil
		}}
}

func (r *tidyRig) tidy(target int64) Tidy {
	return Tidy{TargetMB: target, FreeMB: func() (int64, error) { return r.free, nil }, Steps: []Step{
		r.step("history", Item{Step: "history", ID: "run_1", Bytes: 300 << 20}),
		r.step("cache", Item{Step: "cache", ID: "entry_a", Bytes: 500 << 20}, Item{Step: "cache", ID: "entry_b", Bytes: 200 << 20}),
		r.step("orphans", Item{Step: "orphans", ID: "job-root-9", Bytes: 100 << 20}),
	}}
}

func TestTidyReportOnlyByDefault(t *testing.T) {
	r := &tidyRig{free: 1000, applied: map[string][]Item{}}
	rep, err := r.tidy(1800).Run(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.applied) != 0 || r.free != 1000 {
		t.Fatalf("a report-only tidy removed something: %+v", r.applied)
	}
	if rep.Apply || len(rep.Items) != 4 || rep.FreeAfterMB != 1000 || !strings.Contains(rep.Why, "--apply would remove 4 item(s)") {
		t.Fatalf("the report must list exactly what --apply would remove: %+v", rep)
	}
}

func TestTidyApplyReachesTarget(t *testing.T) {
	r := &tidyRig{free: 1000, applied: map[string][]Item{}}
	plan, _ := r.tidy(1800).Run(context.Background(), false)
	rep, err := r.tidy(1800).Run(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	var applied []Item
	for _, name := range []string{"history", "cache", "orphans"} {
		applied = append(applied, r.applied[name]...)
	}
	if !reflect.DeepEqual(applied, plan.Items) {
		t.Fatalf("apply must remove exactly the planned items:\n got %+v\nwant %+v", applied, plan.Items)
	}
	if !rep.Reached || rep.FreeBeforeMB != 1000 || rep.FreeAfterMB != 2100 || rep.Why != "" {
		t.Fatalf("the tidy must reach the target: %+v", rep)
	}
	// A target out of reach is reported with why, never claimed.
	r2 := &tidyRig{free: 1000, applied: map[string][]Item{}}
	far, err := r2.tidy(5000).Run(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if far.Reached || !strings.Contains(far.Why, "below the 5000 MB target") {
		t.Fatalf("an unreachable target must say why: %+v", far)
	}
}
