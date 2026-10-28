package pairing

// Purpose: a failed node_key_proof leaves an audit row and an alert, within a budget (D-1258).
// Inputs:  forged requests against a rig. Outputs: pass/fail. Constraints: fake clock, no sleeping.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

func TestProofFailureIsAuditedAndAlerted(t *testing.T) {
	r := newRig(t, nil)
	off := r.offer(OfferOptions{Approver: approving("policy:p", DefaultTrust())})
	atk := newNodeMat(t)
	q := r.request(off.Code, "build-mac", atk)
	q.NodeKeySig = nil
	if _, err := r.svc.Redeem(context.Background(), q, "203.0.113.9:4444"); pki.ReasonOf(err) != pki.ReasonNodeKeyProof {
		t.Fatalf("want node_key_proof, got %v", err)
	}
	rows := r.audit("pairing.refuse")
	if len(rows) != 1 || rows[0].Target != "build-mac" || !strings.Contains(rows[0].Detail, "reason=node_key_proof") || !strings.Contains(rows[0].Detail, "src=203.0.113.9:4444") {
		t.Fatalf("audit rows %+v", rows)
	}
	if got := r.alerts.all(); len(got) != 1 || got[0].Kind != "pairing.proof_failed" || got[0].NodeID != "build-mac" {
		t.Fatalf("alerts %+v", got)
	}
	r.noCodeInAudit(off.Code)
	r.untouched(off) // still not a wrong attempt: the offer is not locked by probing
}

func TestProofFailureRowsAreRateLimited(t *testing.T) {
	r := newRig(t, nil)
	off := r.offer(OfferOptions{Approver: approving("policy:p", DefaultTrust())})
	atk := newNodeMat(t)
	fail := func() {
		q := r.request(off.Code, "build-mac", atk)
		q.NodeKeySig = nil
		_, _ = r.svc.Redeem(context.Background(), q, "203.0.113.9:4444")
	}
	for range proofAuditBurst + 5 {
		fail()
	}
	if n := len(r.audit("pairing.refuse")); n != proofAuditBurst {
		t.Fatalf("%d rows in one window, want %d", n, proofAuditBurst)
	}
	if n := len(r.alerts.all()); n != proofAuditBurst {
		t.Fatalf("%d alerts in one window, want %d", n, proofAuditBurst)
	}
	r.clock.Advance(proofAuditWindow)
	fail()
	rows := r.audit("pairing.refuse")
	if len(rows) != proofAuditBurst+1 || !strings.HasSuffix(rows[len(rows)-1].Detail, "suppressed=5") {
		t.Fatalf("the next window's row must carry the suppressed count: %+v", rows[len(rows)-1])
	}
}

func TestProofAuditorWindow(t *testing.T) {
	var p proofAuditor
	t0 := time.Unix(1000, 0)
	for range proofAuditBurst {
		if ok, _ := p.take(t0); !ok {
			t.Fatal("within budget")
		}
	}
	if ok, _ := p.take(t0.Add(time.Second)); ok {
		t.Fatal("over budget")
	}
	if ok, dropped := p.take(t0.Add(proofAuditWindow)); !ok || dropped != 1 {
		t.Fatalf("new window: ok=%v dropped=%d", ok, dropped)
	}
}
