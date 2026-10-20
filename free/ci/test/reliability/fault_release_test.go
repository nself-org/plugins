//go:build integration

package reliability

// Purpose: a lost release job (and any at-most-once job) is never retried: when its node freezes past the TTL the
//          lease engine sends the attempt to needs-operator, queues no successor, and the thawed node's job is
//          killed before it can write its side effect.

import (
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestFaultLostReleaseJobNeedsAnOperator(t *testing.T) {
	for _, c := range []struct {
		name       string
		kind       model.JobKind
		idempotent bool
	}{{"release", "release", true}, {"at-most-once", "test", false}} {
		t.Run(c.name, func(t *testing.T) {
			l := newLab(t, fast, 2)
			h := l.deliver(0, "rel-"+c.name, c.kind, c.idempotent, jobSpec{iter: 100, name: "rel"})
			until(t, 30*time.Second, "the lease to be renewed", func() bool {
				le, err := l.c.st.LeaseGet(t.Context(), h.lease)
				return err == nil && time.Until(time.Unix(0, le.ExpiresAt)) <= fast.ttl+time.Second
			}, l.diag)
			until(t, 30*time.Second, "the job to beat", func() bool { return l.lines(0, "/tmp/beats") >= 3 }, l.diag)

			l.pause(0)
			until(t, 3*fast.ttl+10*time.Second, "the lost lease to reach needs-operator", func() bool { return l.attemptState(h.attempt) == "needs-operator" }, l.diag)
			l.unpause(0)
			until(t, fast.agentTTL()+10*time.Second, "the thawed node's job to die", func() bool { return !l.jobAlive(0) }, l.diag)
			l.waitConnected(30*time.Second, l.ids[0])

			if got := l.auditDetails("retry-queued"); len(got) != 0 {
				t.Fatalf("a job that must not run twice was queued for a retry: %v", got)
			}
			var n int
			l.c.countAttempts(&n, "j-rel-"+c.name)
			if n != 1 {
				t.Fatalf("%d attempts of the job, want 1 (no successor row)", n)
			}
			if l.fleetSideEffects() != 0 || h.sink.nResults() != 0 {
				t.Fatalf("the lost job wrote its side effect (%d) or delivered a result (%d)", l.fleetSideEffects(), h.sink.nResults())
			}
		})
	}
}
