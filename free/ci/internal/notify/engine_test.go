package notify

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// recorder is a sink that keeps what it was given.
type recorder struct {
	name string
	mu   sync.Mutex
	got  []Event
	err  error
}

func (r *recorder) Name() string { return r.name }
func (r *recorder) Notify(_ context.Context, ev Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, ev)
	return r.err
}
func (r *recorder) kinds() []Kind {
	r.mu.Lock()
	defer r.mu.Unlock()
	var k []Kind
	for _, e := range r.got {
		k = append(k, e.Kind)
	}
	return k
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func testEngine(t *testing.T, sinks ...Notifier) (*Engine, *store.Store, *clock) {
	t.Helper()
	clk := &clock{t: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)}
	st, err := store.Open(t.TempDir()+"/state.db", store.Options{Clock: clk.now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &Engine{Sinks: sinks, State: StoreState{S: st}, Now: clk.now}, st, clk
}

func run(sha, result string) Run {
	return Run{Repo: "r1", Ref: "main", Revision: sha, Context: "nself-ci", RunID: "run-" + sha, Result: result,
		Counts: model.Counts{Pass: 1, Fail: 2, Total: 3}, FailedJobs: []string{"unit"}}
}

func sha(n string) string { return strings.Repeat(n, 40)[:40] }

func TestFailureAlertOncePerCommit(t *testing.T) {
	rec := &recorder{name: "rec"}
	e, _, _ := testEngine(t, rec)
	ctx := context.Background()
	e.OnRun(ctx, run(sha("a"), "fail"))
	e.OnRun(ctx, run(sha("a"), "fail")) // a rerun of the same sha with the same result
	if got := rec.kinds(); len(got) != 1 || got[0] != KindFailure {
		t.Fatalf("one alert for one commit, got %v", got)
	}
	first := rec.got[0]
	if first.Repo != "r1" || first.Revision != sha("a") || first.RunID != "run-"+sha("a") || first.Counts.Fail != 2 || first.Show != "nself ci runs show run-"+sha("a") || !strings.Contains(first.Message, "2 of 3") {
		t.Fatalf("event: %+v", first)
	}
	e.OnRun(ctx, run(sha("b"), "fail")) // another commit
	other := run(sha("a"), "fail")
	other.Context = "lint" // another context of the same commit
	e.OnRun(ctx, other)
	if got := rec.kinds(); len(got) != 3 {
		t.Fatalf("a new sha and a new context each alert once: %v", got)
	}
	// an infrastructure error is its own kind, once per commit
	e.OnRun(ctx, run(sha("c"), "error"))
	e.OnRun(ctx, run(sha("c"), "error"))
	if got := rec.kinds(); len(got) != 4 || got[3] != KindErrorStuck {
		t.Fatalf("error_stuck once: %v", got)
	}
	// a cancelled run and a quiet kind send nothing
	e.OnRun(ctx, run(sha("d"), "cancelled"))
	e.Quiet = map[Kind]bool{KindFailure: true}
	e.OnRun(ctx, run(sha("e"), "fail"))
	if got := rec.kinds(); len(got) != 4 {
		t.Fatalf("cancelled and quiet send nothing: %v", got)
	}
}

func TestFailureAlertRetriedWhenNoSinkTookIt(t *testing.T) {
	bad := &recorder{name: "bad", err: errors.New("down")}
	e, st, _ := testEngine(t, bad)
	ctx := context.Background()
	notes := e.OnRun(ctx, run(sha("a"), "fail"))
	if len(notes) == 0 || !strings.Contains(notes[0], "sink bad did not take") || strings.Contains(notes[0], "down") {
		t.Fatalf("a failed delivery is a note naming the sink only: %v", notes)
	}
	bad.err = nil
	e.OnRun(ctx, run(sha("a"), "fail")) // nobody took it the first time, so the rerun may alert
	if len(bad.got) != 2 {
		t.Fatalf("an untaken alert is released: %d", len(bad.got))
	}
	stats, _ := st.NotifySinkStats(ctx)
	if len(stats) != 1 || stats[0].Failed != 1 || stats[0].Delivered != 1 {
		t.Fatalf("counters: %+v", stats)
	}
}

func TestRecoveryMessage(t *testing.T) {
	rec := &recorder{name: "rec"}
	e, _, _ := testEngine(t, rec)
	ctx := context.Background()
	steps := []struct {
		run  Run
		want int // events so far
	}{
		{run(sha("a"), "pass"), 0}, // green with no red before: nothing
		{run(sha("b"), "fail"), 1}, // red: failure
		{run(sha("c"), "pass"), 2}, // red -> green on the same ref: recovery
		{run(sha("c"), "pass"), 2}, // a rerun of the green sha: nothing
		{run(sha("d"), "pass"), 2}, // still green: nothing
	}
	for i, s := range steps {
		e.OnRun(ctx, s.run)
		if got := len(rec.kinds()); got != s.want {
			t.Fatalf("step %d: %d events, want %d (%v)", i, got, s.want, rec.kinds())
		}
	}
	if rec.got[1].Kind != KindRecovery || rec.got[1].Revision != sha("c") || !strings.Contains(rec.got[1].Message, "green again on r1@main") {
		t.Fatalf("recovery event: %+v", rec.got[1])
	}
	// red on a topic branch does not make a green main a recovery
	topic := run(sha("e"), "fail")
	topic.Ref = "topic"
	e.OnRun(ctx, topic)
	e.OnRun(ctx, run(sha("f"), "pass"))
	if got := rec.kinds(); len(got) != 3 || got[2] != KindFailure {
		t.Fatalf("recovery needs the same ref: %v", got)
	}
	// error -> green is a recovery too
	e.OnRun(ctx, run(sha("g"), "error"))
	e.OnRun(ctx, run(sha("h"), "pass"))
	if got := rec.kinds(); len(got) != 5 || got[4] != KindRecovery {
		t.Fatalf("recovery after an infrastructure error: %v", got)
	}
}

// A recovery nobody heard is not lost: the red result is put back, so the next green run sends it.
func TestRecoveryRetriedWhenNoSinkTookIt(t *testing.T) {
	rec := &recorder{name: "rec"}
	e, _, _ := testEngine(t, rec)
	ctx := context.Background()
	e.OnRun(ctx, run(sha("a"), "fail"))
	rec.err = errors.New("down")
	e.OnRun(ctx, run(sha("b"), "pass")) // recovery goes nowhere
	rec.err = nil
	e.OnRun(ctx, run(sha("c"), "pass"))
	if got := rec.kinds(); len(got) != 3 || got[2] != KindRecovery || rec.got[2].Revision != sha("c") {
		t.Fatalf("the recovery must be sent by the next green run: %v", got)
	}
	e.OnRun(ctx, run(sha("d"), "pass"))
	if len(rec.got) != 3 {
		t.Fatal("and only once")
	}
}

// A run with no ref (detached checkout) alerts on failure but never claims a recovery; a run with no commit sends nothing.
func TestDetachedAndCommitlessRuns(t *testing.T) {
	rec := &recorder{name: "rec"}
	e, _, _ := testEngine(t, rec)
	ctx := context.Background()
	d := run(sha("a"), "fail")
	d.Ref = ""
	e.OnRun(ctx, d)
	d2 := run(sha("b"), "pass")
	d2.Ref = ""
	e.OnRun(ctx, d2)
	none := run("", "fail")
	e.OnRun(ctx, none)
	if got := rec.kinds(); len(got) != 1 || got[0] != KindFailure {
		t.Fatalf("detached: failure only, no recovery: %v", got)
	}
}

// A hung sink that used the budget keeps the digest for a later run.
func TestDigestWaitsAfterATimeout(t *testing.T) {
	hang := &blocker{name: "hang", started: make(chan struct{}), cancelled: make(chan struct{})}
	e, _, _ := testEngine(t, hang)
	e.DailyDigest = true
	fire := make(chan time.Time, 1)
	e.After = func(time.Duration) <-chan time.Time { return fire }
	fire <- time.Time{}
	var asked int
	e.State = countingState{State: e.State, digestAsked: &asked}
	e.OnRun(context.Background(), run(sha("a"), "fail"))
	if asked != 0 {
		t.Fatal("the digest must not start after a delivery round that timed out")
	}
}

type countingState struct {
	State
	digestAsked *int
}

func (c countingState) LastDigest(ctx context.Context) (int64, error) {
	*c.digestAsked++
	return c.State.LastDigest(ctx)
}

func TestDailyDigest(t *testing.T) {
	rec := &recorder{name: "rec"}
	e, st, clk := testEngine(t, rec)
	e.DailyDigest = true
	ctx := context.Background()
	finish := func(id, status string) {
		t.Helper()
		if err := st.CreatePipeline(ctx, store.PipelineRow{ID: id, Revision: "rev", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
			t.Fatal(err)
		}
		if err := st.FinishPipeline(ctx, id, status); err != nil {
			t.Fatal(err)
		}
	}
	digests := func() []Event {
		var out []Event
		for _, ev := range rec.got {
			if ev.Kind == KindDigest {
				out = append(out, ev)
			}
		}
		return out
	}
	e.OnRun(ctx, run(sha("a"), "pass")) // no finished run in the window yet: no digest
	if len(digests()) != 0 {
		t.Fatal("an empty window sends no digest")
	}
	finish("p1", "passed")
	finish("p2", "failed")
	finish("p3", "error")
	clk.t = clk.t.Add(time.Minute)
	e.OnRun(ctx, run(sha("a"), "pass"))
	d := digests()
	if len(d) != 1 || d[0].Digest.Runs != 3 || d[0].Digest.Passed != 1 || d[0].Digest.Failed != 1 || d[0].Digest.Errors != 1 {
		t.Fatalf("first digest: %+v", d)
	}
	clk.t = clk.t.Add(time.Second)
	finish("p4", "passed")
	e.OnRun(ctx, run(sha("a"), "pass")) // same UTC day: once
	if len(digests()) != 1 {
		t.Fatalf("one digest per day: %d", len(digests()))
	}
	clk.t = clk.t.Add(24 * time.Hour) // next day
	finish("p5", "failed")
	e.OnRun(ctx, run(sha("a"), "pass"))
	d = digests()
	if len(d) != 2 || d[1].Digest.Runs != 2 || !strings.Contains(d[1].Message, "2 runs") {
		t.Fatalf("second digest covers what finished since the first: %+v", d)
	}
	e.Quiet = map[Kind]bool{KindDigest: true}
	clk.t = clk.t.Add(24 * time.Hour)
	finish("p6", "passed")
	e.OnRun(ctx, run(sha("a"), "pass"))
	if len(digests()) != 2 {
		t.Fatal("a quiet digest is not sent")
	}
}

// A hanging sink is cut off at the budget: the call returns, the sink is counted as failed,
// and the other sink still delivered.
func TestHangingSinkIsBounded(t *testing.T) {
	good := &recorder{name: "good"}
	hang := &blocker{name: "hang", started: make(chan struct{}), cancelled: make(chan struct{})}
	e, st, _ := testEngine(t, hang, good)
	fire := make(chan time.Time)
	e.After = func(time.Duration) <-chan time.Time { return fire }
	done := make(chan []string)
	go func() { done <- e.OnRun(context.Background(), run(sha("a"), "fail")) }()
	<-hang.started
	for deadline := time.Now().Add(30 * time.Second); ; runtime.Gosched() { // the budget ends only after the good sink's delivery was counted
		if stats, _ := st.NotifySinkStats(context.Background()); len(stats) == 1 && stats[0].Sink == "good" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the good sink's delivery was never counted")
		}
	}
	fire <- time.Time{}
	notes := <-done
	if len(good.kinds()) != 1 || len(notes) != 1 || !strings.Contains(notes[0], "sink hang") {
		t.Fatalf("good %d notes %v", len(good.kinds()), notes)
	}
	stats, _ := st.NotifySinkStats(context.Background())
	if len(stats) != 2 || stats[0].Sink != "good" || stats[0].Delivered != 1 || stats[1].Sink != "hang" || stats[1].Failed != 1 {
		t.Fatalf("counters: %+v", stats)
	}
	<-hang.cancelled // the budget cancelled the sink's context
}

type blocker struct {
	name               string
	started, cancelled chan struct{}
}

func (b *blocker) Name() string { return b.name }
func (b *blocker) Notify(ctx context.Context, _ Event) error {
	close(b.started)
	<-ctx.Done()
	close(b.cancelled)
	return ctx.Err()
}

type panicker struct{}

func (panicker) Name() string                        { return "boom" }
func (panicker) Notify(context.Context, Event) error { panic("sink bug") }

func TestPanickingSinkIsAFailedDelivery(t *testing.T) {
	good := &recorder{name: "good"}
	e, _, _ := testEngine(t, panicker{}, good)
	notes := e.OnRun(context.Background(), run(sha("a"), "fail"))
	if len(good.got) != 1 || len(notes) != 1 || !strings.Contains(notes[0], "sink boom") {
		t.Fatalf("good %d notes %v", len(good.got), notes)
	}
}

func TestEventRedactedAndBounded(t *testing.T) {
	token := "gh" + "p_" + strings.Repeat("A", 36)
	ev := Event{Kind: KindFailure, Message: "failed with " + token, Repo: "r\x00x", FailedJobs: make([]string, 30)}
	worst := Event{Kind: KindFailure, Message: strings.Repeat("<", 400), Repo: strings.Repeat("&", 400), Ref: strings.Repeat(">", 400), Revision: strings.Repeat("\u2028", 400), Context: strings.Repeat("<", 400), RunID: strings.Repeat("<", 400), Show: strings.Repeat("<", 400)}
	for i := 0; i < maxFailedJobs; i++ {
		worst.FailedJobs = append(worst.FailedJobs, strings.Repeat("&", 400))
	}
	if b, _ := json.Marshal(worst.Redacted()); len(b) > MaxBody {
		t.Fatalf("a worst-case event is %d bytes, over the %d cap", len(b), MaxBody)
	}
	for i := range ev.FailedJobs {
		ev.FailedJobs[i] = strings.Repeat("j", 500)
	}
	got := ev.Redacted()
	if strings.Contains(got.Message, token) || strings.ContainsRune(got.Repo, 0) || len(got.FailedJobs) != maxFailedJobs || len([]rune(got.FailedJobs[0])) > maxField {
		t.Fatalf("redacted event: %+v", got)
	}
}
