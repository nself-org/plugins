package schedule

// Purpose: tests of the schedule Reconciler and the operator-scope key (P7-HOST-20, acceptance lines
// 2 and 3).
// Inputs:  a mutable test clock, a real SQLite store in a temp dir (marks and intake), a fake head
//          resolver, entries built in the test.
// Outputs: pass or fail.
// Constraints: no network, no real time, every revision synthetic.

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const testRev = "0123456789abcdef0123456789abcdef01234567"

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

type heads struct {
	rev   string
	err   error
	calls int
}

func (h *heads) Head(context.Context, string, string) (string, error) {
	h.calls++
	return h.rev, h.err
}

// recorder is the Enqueuer of the fire-count tests.
type recorder struct{ got []coordinator.Delivery }

func (r *recorder) Enqueue(_ context.Context, d coordinator.Delivery) (coordinator.Outcome, error) {
	r.got = append(r.got, d)
	return coordinator.Outcome{Kind: store.IntakeEnqueued}, nil
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func entries(t *testing.T, cron string) func(context.Context) ([]Entry, error) {
	t.Helper()
	v, err := Decode([]byte(`[{"repo":"acme/app","pipeline":"nightly","cron":"` + cron + `"}]`))
	if err != nil {
		t.Fatal(err)
	}
	return func(context.Context) ([]Entry, error) { return v.([]Entry), nil }
}

func rig(t *testing.T, cron, start string) (*Reconciler, *clock, *heads, *store.Store) {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	clk, h, st := &clock{now: at(start)}, &heads{rev: testRev}, openStore(t)
	return &Reconciler{Entries: entries(t, cron), Marks: st, Heads: h, Clock: clk.Now}, clk, h, st
}

// TestFiresOncePerTime proves acceptance line 2, first half: with `0 * * * *` and 5-minute ticks over
// 3 hours of injected clock exactly 3 fires occur, each carrying the scheduled time in its delivery id.
func TestFiresOncePerTime(t *testing.T) {
	r, clk, _, _ := rig(t, "0 * * * *", "2026-10-19 10:02")
	rec := &recorder{}
	for end := clk.now.Add(3 * time.Hour); !clk.now.After(end); clk.now = clk.now.Add(Every) {
		if err := r.Reconcile(context.Background(), rec); err != nil {
			t.Fatal(err)
		}
	}
	if len(rec.got) != 3 {
		t.Fatalf("%d fires over 3 hours, want 3", len(rec.got))
	}
	for i, hour := range []string{"11", "12", "13"} {
		d := rec.got[i]
		want := "schedule:acme/app:nightly:2026-10-19T" + hour + ":00:00Z"
		if d.DeliveryID != want || d.Source != "schedule" || d.TriggerClass != "schedule" || d.Project != "acme/app" || d.Pipeline != "nightly" || d.Revision != testRev {
			t.Errorf("fire %d = %+v, want delivery id %s", i, d, want)
		}
		var f model.TriggerFacts
		if err := json.Unmarshal(d.Facts, &f); err != nil || f.Validate() != nil || f.Kind != "schedule" || f.Source != "schedule" || f.Auth != "per-repo" || f.DeliveryID != want {
			t.Errorf("fire %d facts = %+v (%v)", i, f, err)
		}
	}
}

// TestMissedCollapse proves acceptance line 2, second half: after a 5-hour gap one fire occurs for the
// missed times, and it names the latest of them. The first sight of an entry fires nothing.
func TestMissedCollapse(t *testing.T) {
	r, clk, h, _ := rig(t, "0 * * * *", "2026-10-19 10:02")
	rec := &recorder{}
	if err := r.Reconcile(context.Background(), rec); err != nil || len(rec.got) != 0 || h.calls != 0 {
		t.Fatalf("first sight: fires=%d head calls=%d err=%v, want none", len(rec.got), h.calls, err)
	}
	clk.now = clk.now.Add(5 * time.Hour) // coordinator down: 11:00..15:00 missed
	if err := r.Reconcile(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.got) != 1 || rec.got[0].DeliveryID != "schedule:acme/app:nightly:2026-10-19T15:00:00Z" {
		t.Fatalf("fires after the gap = %+v, want one for 15:00", rec.got)
	}
	clk.now = clk.now.Add(Every)
	if err := r.Reconcile(context.Background(), rec); err != nil || len(rec.got) != 1 {
		t.Fatalf("next tick refired: %d fires, err=%v", len(rec.got), err)
	}
}

type failMarks struct {
	*store.Store
	failPuts int
}

func (m *failMarks) ProviderMarkPut(ctx context.Context, name, key string, at time.Time) error {
	if m.failPuts > 0 {
		m.failPuts--
		return errors.New("disk full")
	}
	return m.Store.ProviderMarkPut(ctx, name, key, at)
}

// TestDedup proves acceptance line 3, first half: a duplicate tick for the same scheduled time (here a
// crash between the enqueue and the mark write) sends the same delivery id again and the real intake
// stores nothing new. A failed head lookup or enqueue keeps the mark, so the time is retried.
func TestDedup(t *testing.T) {
	r, clk, h, st := rig(t, "0 * * * *", "2026-10-19 10:02")
	c, err := coordinator.New(coordinator.Config{Store: st, Insecure: true})
	if err != nil {
		t.Fatal(err)
	}
	marks := &failMarks{Store: st}
	r.Marks = marks
	if err = r.Reconcile(context.Background(), c); err != nil { // first sight
		t.Fatal(err)
	}
	clk.now = at("2026-10-19 11:05")
	h.err = errors.New("provider down")
	if err = r.Reconcile(context.Background(), c); err == nil || !strings.Contains(err.Error(), "resolve head") {
		t.Fatalf("head failure = %v", err)
	}
	h.err = nil
	marks.failPuts = 1
	if err = r.Reconcile(context.Background(), c); err == nil { // enqueued, mark write failed
		t.Fatal("mark failure must surface")
	}
	if got := queued(t, st); got != 1 {
		t.Fatalf("%d rows after the first enqueue, want 1", got)
	}
	clk.now = at("2026-10-19 11:10")
	rec := &outcomes{Enqueuer: c}
	if err = r.Reconcile(context.Background(), rec); err != nil { // same scheduled time again
		t.Fatal(err)
	}
	if len(rec.kinds) != 1 || rec.kinds[0] != store.IntakeDuplicate || queued(t, st) != 1 {
		t.Fatalf("duplicate tick: outcomes=%v rows=%d, want one duplicate and one row", rec.kinds, queued(t, st))
	}
	clk.now = at("2026-10-19 11:15")
	if err = r.Reconcile(context.Background(), rec); err != nil || len(rec.kinds) != 1 {
		t.Fatalf("mark did not advance: %v %v", rec.kinds, err)
	}
}

type outcomes struct {
	coordinator.Enqueuer
	kinds []string
}

func (o *outcomes) Enqueue(ctx context.Context, d coordinator.Delivery) (coordinator.Outcome, error) {
	out, err := o.Enqueuer.Enqueue(ctx, d)
	o.kinds = append(o.kinds, out.Kind)
	return out, err
}

func queued(t *testing.T, st *store.Store) int {
	t.Helper()
	s, err := st.IntakeStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s.Queued
}

// TestOperatorScopeOnly proves acceptance line 3, second half: triggers.schedules in a revision's
// .nself/ci.yaml `policy:` (or the protected branch, pipeline, job) is ignored with E671 and no
// schedule results; an operator-scope value is used; the Reconciler is dormant without V15 and a nil
// policy is an error rather than "no schedules".
func TestOperatorScopeOnly(t *testing.T) {
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	got, ok := policy.Default.Find(KeySchedules)
	if !ok || !got.Spend || got.Kind != policy.TightenFixed {
		t.Fatalf("registered=%v operator-only=%v kind=%v", ok, got.Spend, got.Kind)
	}
	reg, err := trust.NewRegistry(Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	merge := func(kind trust.Source, raw string) (trust.Effective, error) {
		eff, _, err := reg.Merge("", trust.Scope{Kind: kind, Values: map[string]json.RawMessage{KeySchedules: json.RawMessage(raw)}})
		return eff, err
	}
	evil := `[{"repo":"acme/app","pipeline":"nightly","cron":"* * * * *"}]`
	for _, kind := range []trust.Source{trust.SourceRevision, trust.SourceProtectedBranch, trust.SourcePipeline, trust.SourceJob} {
		eff, err := merge(kind, evil)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		list, err := EntriesFrom(&eff)
		if err != nil || len(list) != 0 {
			t.Fatalf("%s scope scheduled %v (%v)", kind, list, err)
		}
		ig := eff.Ignored()
		if len(ig) != 1 || ig[0].Key != KeySchedules || !strings.HasPrefix(ig[0].Reason, "E671") {
			t.Errorf("%s: ignored = %+v, want %s with E671", kind, ig, KeySchedules)
		}
	}
	eff, err := merge(trust.SourceUser, evil)
	if err != nil {
		t.Fatal(err)
	}
	list, err := EntriesFrom(&eff)
	if err != nil || len(list) != 1 || list[0].Repo != "acme/app" || len(eff.Ignored()) != 0 {
		t.Fatalf("operator scope: %v %v ignored=%v", list, err, eff.Ignored())
	}
	if _, err = EntriesFrom(nil); err == nil {
		t.Fatal("a nil policy is an error, never no schedules")
	}
	if _, err = merge(trust.SourceUser, `[{"repo":"acme/app","cron":"@daily"}]`); err == nil || !strings.Contains(err.Error(), "E672") {
		t.Fatalf("bad operator value: %v, want E672", err)
	}
	UsePolicy(func(context.Context) (*policy.Effective, error) { return &eff, nil })
	t.Cleanup(func() { UsePolicy(nil) })
	if cur, err := CurrentEntries(context.Background()); err != nil || len(cur) != 1 {
		t.Fatalf("CurrentEntries = %v %v", cur, err)
	}
	t.Setenv("NSELF_V15", "")
	rec := &recorder{}
	if err = (&Reconciler{Entries: CurrentEntries}).Reconcile(context.Background(), rec); err != nil || len(rec.got) != 0 {
		t.Fatalf("dormant without V15: %v %v", rec.got, err)
	}
}

type failEnq struct {
	fail bool
	got  []string
}

func (f *failEnq) Enqueue(_ context.Context, d coordinator.Delivery) (coordinator.Outcome, error) {
	if f.fail {
		return coordinator.Outcome{}, errors.New("intake down")
	}
	f.got = append(f.got, d.DeliveryID)
	return coordinator.Outcome{Kind: store.IntakeEnqueued}, nil
}

// TestFailureKeepsMark: an enqueue error keeps the mark, so the time is retried on the next tick; and
// one failing entry never stops the others in the same pass.
func TestFailureKeepsMark(t *testing.T) {
	r, clk, _, _ := rig(t, "0 * * * *", "2026-10-19 10:02")
	r.Entries = func(context.Context) ([]Entry, error) {
		v, err := Decode([]byte(`[{"repo":"acme/app","pipeline":"nightly","cron":"0 * * * *"},{"repo":"acme/other","cron":"0 * * * *"}]`))
		return v.([]Entry), err
	}
	enq := &failEnq{}
	if err := r.Reconcile(context.Background(), enq); err != nil { // first sight of both
		t.Fatal(err)
	}
	clk.now = at("2026-10-19 11:05")
	enq.fail = true
	if err := r.Reconcile(context.Background(), enq); err == nil || strings.Count(err.Error(), "intake down") != 2 {
		t.Fatalf("both entries must be tried and both errors returned: %v", err)
	}
	enq.fail = false
	clk.now = at("2026-10-19 11:10")
	if err := r.Reconcile(context.Background(), enq); err != nil || len(enq.got) != 2 || !strings.HasSuffix(enq.got[0], "2026-10-19T11:00:00Z") {
		t.Fatalf("retry fired %v (%v), want both entries for 11:00", enq.got, err)
	}
	clk.now = at("2026-10-19 11:15")
	if err := r.Reconcile(context.Background(), enq); err != nil || len(enq.got) != 2 {
		t.Fatalf("refired after success: %v %v", enq.got, err)
	}
	clk.now = at("2026-10-19 09:00") // a clock that goes backwards fires nothing
	if err := r.Reconcile(context.Background(), enq); err != nil || len(enq.got) != 2 {
		t.Fatalf("backwards clock fired: %v %v", enq.got, err)
	}
}
