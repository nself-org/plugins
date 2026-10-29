package run

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/transport"
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

// TestHostedLeaseNeverGetsSnapshot: with a dirty-tree snapshot in the run, a lease
// delivered to an enrolled node checks out the snapshot, while a lease delivered to a
// hosted runner checks out the committed HEAD (a hosted runner fetches from the
// forge, where no snapshot exists), its evidence must name HEAD, and the run notes
// say the dirty changes were not included. A run without a snapshot is unchanged.
// The run engine itself does not place a foreground dev run on a hosted runner today
// (dropProviderRunners, and privacy.hosted_not_authorized outside a public
// repository), so the delivery boundary, where Hosted is known, is what is tested.
func TestHostedLeaseNeverGetsSnapshot(t *testing.T) {
	const head, snap = "1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222"
	probe := &probeDriver{kind: fmt.Sprintf("offloadprobe%d", time.Now().UnixNano())}
	if err := transport.Register(probe); err != nil {
		t.Fatal(err)
	}
	var node model.Capability
	node.Identity.ID, node.Identity.Transport = "x1", probe.kind
	deliver := func(t *testing.T, rd *runDispatch, attempt, revision string, hosted bool) (lease, expected string) {
		t.Helper()
		rd.sink.expect(attempt, revision)
		d := deliveryFor(node, model.Job{Kind: "test", Run: []string{"true"}})
		d.AttemptID, d.LeaseID, d.Revision, d.Hosted = attempt, "lease-"+attempt, revision, hosted
		ad := transportAdapter{hosted: rd.hostedRevision}
		if err := ad.Deliver(context.Background(), d); err != nil {
			t.Fatal(err)
		}
		probe.mu.Lock()
		defer probe.mu.Unlock()
		return probe.lease.Checkout.Revision, rd.sink.attempt(attempt).revision
	}
	newRD := func(id, checkout string) *runDispatch {
		rd := &runDispatch{sess: &Session{RunID: id, Revision: head, Checkout: checkout}, sink: &remoteSink{by: map[string]*remoteAttempt{}}}
		runDispatchers.Store(id, rd)
		t.Cleanup(func() { runDispatchers.Delete(id) })
		return rd
	}

	rd := newRD("r-offload-hosted", snap)
	if lease, want := deliver(t, rd, "a1", snap, false); lease != snap || want != snap {
		t.Errorf("node lease %s expects %s: want the snapshot", lease, want)
	}
	if n := dispatchNotes("r-offload-hosted"); len(n) != 0 {
		t.Errorf("a node placement noted %v", n)
	}
	if lease, want := deliver(t, rd, "a2", snap, true); lease != head || want != head {
		t.Errorf("hosted lease %s expects %s: want HEAD %s, never the snapshot", lease, want, head)
	}
	deliver(t, rd, "a3", snap, true)
	if n := dispatchNotes("r-offload-hosted"); len(n) != 1 || n[0] != HostedDirtyNote {
		t.Errorf("notes %v, want the hosted note once", n)
	}

	clean := newRD("r-offload-clean", "")
	if lease, want := deliver(t, clean, "a1", head, true); lease != head || want != head || len(dispatchNotes("r-offload-clean")) != 0 {
		t.Errorf("a run without a snapshot: hosted lease %s expects %s notes %v", lease, want, dispatchNotes("r-offload-clean"))
	}
}
