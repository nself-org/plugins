package run

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// TestRemoteAdapterCarriesEvidenceContext checks the production adapter against
// the same ledger read by the remote verifier, rather than guessed job/run names.
func TestRemoteAdapterCarriesEvidenceContext(t *testing.T) {
	e := newRemoteEnv(t)
	ctx := context.Background()
	probe := &probeDriver{kind: fmt.Sprintf("evidence-context-%d", time.Now().UnixNano())}
	if err := transport.Register(probe); err != nil {
		t.Fatal(err)
	}
	var node model.Capability
	node.Identity.ID, node.Identity.Transport = "n1", probe.kind
	d := deliveryFor(node, model.Job{Kind: "test", Run: []string{"true"}})
	d.AttemptID, d.LeaseID, d.Epoch = e.id, e.lease, e.epoch
	if err := newTransportAdapter(e.sink, nil, "").Deliver(ctx, d); err != nil {
		t.Fatal(err)
	}
	f, ok, err := (store.AttestLedger{S: e.st}).AttemptFacts(ctx, e.id)
	if err != nil || !ok {
		t.Fatalf("ledger: %+v, %v", f, err)
	}
	probe.mu.Lock()
	got := probe.lease.Evidence
	probe.mu.Unlock()
	want := protocol.EvidenceContext{RunID: f.RunID, RepoID: f.RepoID, SourceTrust: model.TrustClass(f.SourceTrust),
		PolicyDigest: f.PolicyDigest, SelectionMode: model.SelectionMode(f.SelectionMode), Job: f.Job, AttemptN: 1}
	if got == nil || *got != want {
		t.Fatalf("lease evidence context = %+v, want %+v", got, want)
	}
}

func TestRemoteEvidenceContextRefusesUnknownAttempts(t *testing.T) {
	e := newRemoteEnv(t)
	for _, id := range []string{"missing", "run1.jobA.a2"} {
		t.Run(id, func(t *testing.T) {
			if got, err := remoteEvidenceContext(context.Background(), e.sink, id); err == nil || got != nil {
				t.Fatalf("unknown attempt produced context: %+v, %v", got, err)
			}
		})
	}
}
