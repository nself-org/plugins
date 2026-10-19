package pairing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// D-1258: a request that fails node_key_proof returns before the claim; it must still leave a
// rate-limited audit row and alert so takeover probing is visible.
func TestProofFailureIsAuditedAndRateLimited(t *testing.T) {
	r := newRig(t, nil)
	off := r.offer(OfferOptions{Approver: approving("policy:p", DefaultTrust())})
	probe := func() {
		t.Helper()
		m := newNodeMat(t)
		q := r.request(off.Code, "probed-node", m)
		q.NodeKeySig = nil
		if _, err := r.svc.Redeem(context.Background(), q, "203.0.113.9:4444"); pki.ReasonOf(err) != pki.ReasonNodeKeyProof {
			t.Fatalf("want node_key_proof, got %v", err)
		}
	}
	probe()
	rows := r.audit("pairing.proof_failed")
	if len(rows) != 1 || rows[0].Target != "probed-node" || !strings.Contains(rows[0].Detail, "count=1") || !strings.Contains(rows[0].Detail, "203.0.113.9") {
		t.Fatalf("first failure audit rows: %+v", rows)
	}
	if a := r.alerts.all(); len(a) != 1 || a[0].Kind != "pairing.proof_failed" {
		t.Fatalf("alerts: %+v", a)
	}
	for i := 0; i < 5; i++ {
		probe()
	}
	if got := len(r.audit("pairing.proof_failed")); got != 1 {
		t.Fatalf("a flood wrote %d rows; the limit is one per %v", got, ProofAuditEvery)
	}
	r.clock.Advance(ProofAuditEvery + time.Second)
	probe()
	rows = r.audit("pairing.proof_failed")
	if len(rows) != 2 || !strings.Contains(rows[1].Detail, "count=6") {
		t.Fatalf("after the window: %+v", rows)
	}
	r.untouched(off)
}
