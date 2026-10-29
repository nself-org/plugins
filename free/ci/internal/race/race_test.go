package race

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeNode is one runner: a sibling's process group lives until it ends.
type fakeNode struct {
	alive bool
	end   chan Result
	// onCancel is what the attempt reports when cancelled (zero: cancelled).
	onCancel Result
}

// fakeAttempts admits a sibling on the runner it is asked for (or on admit[runner]).
type fakeAttempts struct {
	mu      sync.Mutex
	nodes   map[string]*fakeNode
	admit   map[string]string
	created []Sibling
	cancels map[string]int
	refuse  map[string]error
}

func newFake(runners ...string) *fakeAttempts {
	f := &fakeAttempts{nodes: map[string]*fakeNode{}, admit: map[string]string{}, cancels: map[string]int{}, refuse: map[string]error{}}
	for _, r := range runners {
		f.nodes[r] = &fakeNode{end: make(chan Result, 1)}
	}
	return f
}

func (f *fakeAttempts) Create(_ context.Context, n int, runner string) (Sibling, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.refuse[runner]; err != nil {
		return Sibling{}, err
	}
	if to := f.admit[runner]; to != "" {
		runner = to
	}
	s := Sibling{N: n, AttemptID: "job.a" + string(rune('0'+n)), RunnerID: runner, Epoch: int64(n)}
	f.created = append(f.created, s)
	f.nodes[runner].alive = true
	return s, nil
}

func (f *fakeAttempts) Wait(_ context.Context, s Sibling) Result {
	r := <-f.nodes[s.RunnerID].end
	f.mu.Lock()
	f.nodes[s.RunnerID].alive = false
	f.mu.Unlock()
	return r
}

// Cancel kills the process group: the sibling ends with onCancel or cancelled.
func (f *fakeAttempts) Cancel(_ context.Context, s Sibling, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels[s.AttemptID]++
	n := f.nodes[s.RunnerID]
	r := n.onCancel
	if r.State == "" {
		r = Result{State: StateCancelled, FailureClass: StateCancelled}
	}
	select {
	case n.end <- r:
	default: // already ended: the cancel is a no-op
	}
	return nil
}

func (f *fakeAttempts) finish(runner string, r Result) { f.nodes[runner].end <- r }

type fakePoster struct {
	mu    sync.Mutex
	posts []Evidence
}

func (p *fakePoster) Post(_ context.Context, e Evidence) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.posts = append(p.posts, e)
	return nil
}

var (
	pass    = Result{State: StatePassed}
	codeRed = Result{State: StateFailed, FailureClass: ClassCode, ExitCode: 1}
	infra   = Result{State: StateFailed, FailureClass: "infra"}
)

func choice(runners ...string) Choice { return Choice{Runners: runners, Excluded: map[string]string{}} }

func TestRaceFirstConclusiveWins(t *testing.T) {
	f, p := newFake("n1", "n2"), &fakePoster{}
	f.finish("n1", pass) // n1 is faster; n2 only ends when cancelled
	ev, err := Run(context.Background(), Spec{Job: "job", Choice: choice("n1", "n2"), First: 1}, f, p)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Winner == nil || ev.Winner.RunnerID != "n1" || ev.Result != pass {
		t.Fatalf("winner = %+v result = %+v", ev.Winner, ev.Result)
	}
	if got := ev.CancelledSiblings(); len(got) != 1 || got[0] != "job.a2" {
		t.Fatalf("cancelled = %v", got)
	}
	if f.nodes["n2"].alive {
		t.Fatal("the loser's process group is still alive")
	}
	if f.cancels["job.a1"] != 0 || f.cancels["job.a2"] != 1 {
		t.Fatalf("cancels = %v", f.cancels)
	}
	// a red first finisher is conclusive too
	f, p = newFake("n1", "n2"), &fakePoster{}
	f.finish("n1", codeRed)
	ev, err = Run(context.Background(), Spec{Job: "job", Choice: choice("n1", "n2"), First: 1}, f, p)
	if err != nil || ev.Winner == nil || ev.Winner.RunnerID != "n1" || ev.Result != codeRed || len(ev.CancelledSiblings()) != 1 {
		t.Fatalf("code failure did not end the race: %+v %v", ev, err)
	}
}

func TestRaceInfraNotConclusive(t *testing.T) {
	f, p := newFake("n1", "n2"), &fakePoster{}
	f.finish("n1", infra)
	f.finish("n2", pass)
	ev, err := Run(context.Background(), Spec{Job: "job", Choice: choice("n1", "n2"), First: 1}, f, p)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Winner == nil || ev.Winner.RunnerID != "n2" || ev.Result != pass || ev.Flaky {
		t.Fatalf("an infra error ended the race: %+v", ev)
	}
	// no conclusive result: the race records the first result, with no winner
	f, p = newFake("n1", "n2"), &fakePoster{}
	f.finish("n1", infra)
	f.finish("n2", Result{State: "lost", FailureClass: "lost"})
	ev, err = Run(context.Background(), Spec{Job: "job", Choice: choice("n1", "n2"), First: 1}, f, p)
	if err != nil || ev.Winner != nil || Conclusive(ev.Result) || len(p.posts) != 1 {
		t.Fatalf("inconclusive race: %+v %v posts=%d", ev, err, len(p.posts))
	}
	for _, l := range ev.Losers {
		if l.Disposition != Lost {
			t.Fatalf("loser %+v", l)
		}
	}
}

func TestRaceOneResult(t *testing.T) {
	f, p := newFake("n1", "n2", "n3"), &fakePoster{}
	f.nodes["n2"].onCancel = codeRed // n2 finished red before its cancel landed
	f.nodes["n3"].onCancel = pass    // n3 finished green: same result, superseded
	f.finish("n1", pass)
	ev, err := Run(context.Background(), Spec{Job: "job", Choice: choice("n1", "n2", "n3"), First: 1}, f, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.posts) != 1 {
		t.Fatalf("posts = %d, want exactly one", len(p.posts))
	}
	if ev.Result != pass || !ev.Flaky {
		t.Fatalf("result %+v flaky %v: a later disagreement is data, never the result", ev.Result, ev.Flaky)
	}
	want := map[int]string{2: Flaky, 3: Superseded}
	for _, l := range ev.Losers {
		if want[l.N] != l.Disposition {
			t.Fatalf("sibling %d: %s, want %s", l.N, l.Disposition, want[l.N])
		}
	}
	// a cancelled run posts nothing
	f, p = newFake("n1", "n2"), &fakePoster{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = Run(ctx, Spec{Job: "job", Choice: choice("n1", "n2"), First: 1}, f, p); !errors.Is(err, context.Canceled) || len(p.posts) != 0 {
		t.Fatalf("cancelled run: err %v posts %d", err, len(p.posts))
	}
}

func TestRaceDuplicateRunnerCancelled(t *testing.T) {
	f, p := newFake("n1", "n2"), &fakePoster{}
	f.admit["n2"] = "n1" // the dispatcher admitted the second sibling on n1 as well
	f.finish("n1", pass)
	ev, err := Run(context.Background(), Spec{Job: "job", Choice: choice("n1", "n2"), First: 1}, f, p)
	if err != nil {
		t.Fatal(err)
	}
	if f.cancels["job.a2"] != 1 || len(ev.CancelledSiblings()) != 1 || ev.Winner.AttemptID != "job.a1" {
		t.Fatalf("duplicate sibling kept: %+v cancels %v", ev, f.cancels)
	}
}

func TestRaceNotRacedRefuses(t *testing.T) {
	f, p := newFake("n1"), &fakePoster{}
	_, err := Run(context.Background(), Spec{Job: "job", Choice: Choice{Reason: ReasonRelease}}, f, p)
	if !errors.Is(err, ErrNotRaced) || len(f.created) != 0 || len(p.posts) != 0 {
		t.Fatalf("err %v created %d posts %d", err, len(f.created), len(p.posts))
	}
}
