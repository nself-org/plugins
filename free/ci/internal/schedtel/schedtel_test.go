package schedtel

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func finalize(t *testing.T, run, attempt string, runtimeMs int64) {
	t.Helper()
	info := cirun.RunInfo{RunID: run, Jobs: []cirun.JobOutcome{{AttemptID: attempt, State: cirun.StatePassed, Duration: time.Duration(runtimeMs) * time.Millisecond}}}
	if err := (finalizer{}).Finalize(context.Background(), info); err != nil {
		t.Fatal(err)
	}
}

func ptrVal(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// fiveRuns is the acceptance fixture: runtimes 10, 12, 11, 13, 12 s.
func fiveRuns(t *testing.T, e *env) {
	t.Helper()
	ctx := WithStore(context.Background(), e.st)
	sink := NewSink(e.st, e.ms)
	want := []int64{10000, 10600, 10720, 11404, 11583}
	for i, secs := range []int64{10, 12, 11, 13, 12} {
		run, attempt := e.runOnce(sink, i, secs*1000)
		finalize(t, run, attempt, secs*1000)
		ests, err := e.st.TelEstimators(ctx)
		if err != nil || len(ests) != 1 || ests[0].RuntimeMs != want[i] || ests[0].N != int64(i+1) {
			t.Fatalf("after run %d: %+v (%v), want runtime %d", i+1, ests, err, want[i])
		}
		if got, wantC := Confidence(ests[0].N), map[bool]string{true: "unknown", false: "estimated"}[i < 2]; got != wantC {
			t.Fatalf("after run %d confidence %s, want %s", i+1, got, wantC)
		}
		// what the placement world sees while another attempt waits
		q := e.attempt("queue-"+run, "build")
		w := sched.World{Runners: []sched.Runner{{ID: "local", Class: "local", Location: "local", QueuedAhead: 2, Avail: sched.Avail{Slots: 2}}}}
		if err = contribute(ctx, &w); err != nil {
			t.Fatal(err)
		}
		s, have := w.Estimates["local"][q]
		if i < 2 {
			if have && (s.RuntimeMs != nil || s.ColdStartMs != nil || s.WaitMs != nil || s.InfraFailurePermille != nil) {
				t.Fatalf("run %d: below %d samples nothing is offered: %+v", i+1, MinSamples, s)
			}
			continue
		}
		if !have || ptrVal(s.RuntimeMs) != want[i] || ptrVal(s.ColdStartMs) != int64(500) || ptrVal(s.InfraFailurePermille) != int64(0) ||
			ptrVal(s.WaitMs) != 2*want[i]/2 || s.Confidence != "estimated" {
			t.Fatalf("run %d sample %+v, want runtime %d, cold 500, wait %d", i+1, s, want[i], want[i])
		}
	}
}

func TestTelemetryEWMAFiveRuns(t *testing.T) { fiveRuns(t, newEnv(t)) }

func TestTelemetryEWMAArithmetic(t *testing.T) {
	if got := ewma(0, 7, 0); got != 7 {
		t.Fatalf("first sample is the value: %d", got)
	}
	for _, c := range []struct{ old, sample, want int64 }{{10000, 12000, 10600}, {100, 0, 70}, {5, 6, 5}, {5, 7, 6}} {
		if got := ewma(c.old, c.sample, 1); got != c.want {
			t.Errorf("ewma(%d,%d) = %d, want %d", c.old, c.sample, got, c.want)
		}
	}
}

func TestTelemetryWaitIsMeasuredFromFirstDecision(t *testing.T) {
	e := newEnv(t)
	sink := NewSink(e.st, e.ms)
	run, attempt := e.runOnce(sink, 0, 1000)
	d, ok, err := e.st.TelDecision(context.Background(), attempt)
	if err != nil || !ok || d.WaitMs != 40 || d.Outcome != dispatch.OutcomeAdmitted || d.Run != run || d.RunnerClass != "local" || d.CostMicro != 5 {
		t.Fatalf("decision %+v (%v)", d, err)
	}
}

func TestTelemetryReliabilityUsesLastTwenty(t *testing.T) {
	e := newEnv(t)
	ctx := WithStore(context.Background(), e.st)
	sink := NewSink(e.st, e.ms)
	for i := 0; i < 24; i++ {
		run, attempt := e.runOnce(sink, i, 1000)
		class := ""
		if i < 4 || i == 23 { // 4 old infra failures fall out of the window, the newest stays in
			class = "infra"
		}
		info := cirun.RunInfo{RunID: run, Jobs: []cirun.JobOutcome{{AttemptID: attempt, State: cirun.StateFailed, FailureClass: "infra", Duration: time.Second}}}
		if class == "" {
			info.Jobs[0].State, info.Jobs[0].FailureClass = cirun.StatePassed, ""
		}
		if err := (finalizer{}).Finalize(context.Background(), info); err != nil {
			t.Fatal(err)
		}
		e.advance(time.Second)
	}
	q := e.attempt("queue", "build")
	w := sched.World{Runners: []sched.Runner{{ID: "local", Class: "local", Avail: sched.Avail{Slots: 1}}}}
	if err := contribute(ctx, &w); err != nil {
		t.Fatal(err)
	}
	if got := ptrVal(w.Estimates["local"][q].InfraFailurePermille); got != int64(50) { // 1 of the last 20
		t.Fatalf("infra permille %v, want 50", got)
	}
}

func TestTelemetryNoNetworkEgress(t *testing.T) {
	var dials atomic.Int64
	deny := func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("telemetry must not dial")
	}
	oldRes, oldTr := net.DefaultResolver, http.DefaultTransport
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: deny}
	http.DefaultTransport = &http.Transport{DialContext: deny}
	t.Cleanup(func() { net.DefaultResolver, http.DefaultTransport = oldRes, oldTr })
	e := newEnv(t)
	fiveRuns(t, e)
	src := Source{Open: func(context.Context) (*store.Store, func(), error) { return e.st, func() {}, nil }}
	if _, err := src.Decision(context.Background(), "run-0", "build"); err != nil {
		t.Fatal(err)
	}
	if n := dials.Load(); n != 0 {
		t.Fatalf("%d dial attempts", n)
	}
}

func asPlanviewError(err error, target **planview.Error) bool { return errors.As(err, target) }
