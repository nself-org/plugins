package run

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// TestOffloadCheckoutRevision: a node's lease checks out the snapshot and its
// evidence must name it, while the run's own revision stays the commit
// (P7-AUTO-13, P7-CI-41). Without a snapshot the lease checks out the run's revision.
func TestOffloadCheckoutRevision(t *testing.T) {
	const head, snap = "1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222"
	job := &model.Job{}
	if got := queueJob(&Session{Revision: head, Checkout: snap}, AttemptSpec{Job: job}, sched.Job{}, "a1", 1, 1).Revision; got != snap {
		t.Errorf("lease revision %q, want the snapshot", got)
	}
	if got := queueJob(&Session{Revision: head}, AttemptSpec{Job: job}, sched.Job{}, "a1", 1, 1).Revision; got != head {
		t.Errorf("lease revision %q, want HEAD without a snapshot", got)
	}
	s := &remoteSink{by: map[string]*remoteAttempt{}}
	if ra := s.expect("a1", (&Session{Revision: head, Checkout: snap}).checkoutRevision()); ra.revision != snap {
		t.Errorf("the sink expects %q, want the snapshot the node checked out", ra.revision)
	}
}
