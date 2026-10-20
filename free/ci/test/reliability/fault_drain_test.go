//go:build integration

package reliability

// Purpose: the coordinator's graceful drain with a node it cannot reach. The drain's cancel finds no agent to
//          hear it, the lease expires, the drain hands the attempt back and the run retries it elsewhere. At no
//          moment do two attempts of the job run: the cut-off node's agent fenced its job before the retry began.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/drain"
)

type stopper struct{ stopped atomic.Bool }

func (s *stopper) Stop() { s.stopped.Store(true) }

type cancelFunc func(node, leaseID, reason string) error

func (f cancelFunc) Cancel(node, leaseID, reason string) error { return f(node, leaseID, reason) }

// watchOverlap samples the fleet until stop is called and reports the largest number of nodes that ran the job
// at the same moment.
func (l *lab) watchOverlap() (stop func() (maxAlive int)) {
	done := make(chan struct{})
	res := make(chan int, 1)
	go func() {
		most := 0
		for {
			alive := 0
			for i := range l.names {
				if l.jobAlive(i) {
					alive++
				}
			}
			most = max(most, alive)
			select {
			case <-done:
				res <- most
				return
			case <-time.After(150 * time.Millisecond):
			}
		}
	}()
	return func() int { close(done); return <-res }
}

func TestFaultDrainHandBackNeverOverlaps(t *testing.T) {
	l := newLab(t, fast, 3)
	h := l.start(jobSpec{iter: 100, idempotent: true, infraMax: 1})
	atts := h.waitAttempt(1, 60*time.Second, "leased", "running")
	first := h.runnerOf(atts[0].ID)
	fi := l.index(first)
	h.waitRenewed(atts[0].ID)
	until(t, 30*time.Second, "the job to beat", func() bool { return l.lines(fi, "/tmp/beats") >= 3 }, h.diagText)
	stopOverlap := l.watchOverlap()

	l.partition(fi)
	until(t, 20*time.Second, "the node to be cut off", func() bool { return !l.agentReachable(fi) }, nil)

	intake := &stopper{}
	d, err := drain.New(drain.Config{Intake: intake, Canceller: cancelFunc(l.c.cancel), Deadline: 500 * time.Millisecond, Poll: 100 * time.Millisecond,
		AckTimeout: 30 * time.Second, StopTimeout: 30 * time.Second, Logf: t.Logf,
		Leases: &drain.EngineLeases{Engine: l.c.eng, JobInfo: l.c.st.AttemptJobClass}})
	if err != nil {
		t.Fatal(err)
	}
	ack := func(node, lease string) { d.Ack(lease) }
	l.c.ackHook.Store(&ack)
	rep, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if !intake.stopped.Load() {
		t.Fatal("the drain did not stop intake")
	}
	for _, r := range rep.Results {
		if r.Outcome == drain.OutcomeError || r.Outcome == drain.OutcomeAbandoned {
			t.Fatalf("the drain could not settle lease %s: %s %v", r.LeaseID, r.Outcome, r.Err)
		}
	}
	l.heal(fi)

	res := h.result(2 * time.Minute)
	if res.Exit != 0 || res.Summary.Jobs[0].N != 2 {
		t.Fatalf("exit %d jobs %+v\n%s", res.Exit, res.Summary.Jobs, h.out)
	}
	if most := stopOverlap(); most > 1 {
		t.Fatalf("%d nodes ran the job at the same moment", most)
	}
	if n := l.fleetSideEffects(); n != 1 || l.sideEffects(fi) != 0 {
		t.Fatalf("the side effect was written %d times (%d on the drained node)", n, l.sideEffects(fi))
	}
}
