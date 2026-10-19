package pairing

// Purpose: D-1257, the races of a re-pairing that waits for its approver: a refusal after approval is
// audited and alerted like one before it, the key pin is a compare-and-set against the vetted key, and a
// node removed meanwhile gets nothing.
// Inputs:  a rig, a blocking approver, a SQLite trigger on a second connection.
// Outputs: pass/fail. Constraints: the trigger fires inside the issue transaction, so the race is exact.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

func pairedNode(t *testing.T, r *rig, name string) nodeMat {
	t.Helper()
	m := newNodeMat(t)
	if _, err := r.redeem(r.offer(OfferOptions{Approver: approving("policy:first", DefaultTrust())}).Code, name, m); err != nil {
		t.Fatal(err)
	}
	return m
}

func enrollRevetRefusalIsAudited(t *testing.T) {
	r := newRig(t, nil)
	m1 := pairedNode(t, r, "node-rot")
	ap := &blockingApprover{entered: make(chan struct{}), release: make(chan struct{}), dec: Decision{Approve: true, Trust: DefaultTrust()}}
	off := r.offer(OfferOptions{Approver: ap})
	done := make(chan error, 1)
	go func() { _, err := r.redeem(off.Code, "node-rot", newNodeMat(t).withNodeKeyOf(m1)); done <- err }()
	waitEntered(t, ap)
	if err := r.ca.RotateNodeKey(context.Background(), "admin", "node-rot", pki.NodeKeyID(newNodeMat(t).pub)); err != nil {
		t.Fatal(err)
	}
	close(ap.release)
	if err := waitErr(t, done); pki.ReasonOf(err) != pki.ReasonNodeKeyChanged {
		t.Fatalf("want node_key_changed, got %v", err)
	}
	rows := r.audit("pairing.refuse")
	if len(rows) != 1 || !strings.Contains(rows[0].Detail, "node=node-rot") || !strings.Contains(rows[0].Detail, "reason="+pki.ReasonNodeKeyChanged) {
		t.Fatalf("a refusal after approval left no audit row: %+v", rows)
	}
	if a := r.alerts.all(); len(a) != 1 || a[0].Kind != "pairing.takeover_attempt" || a[0].NodeID != "node-rot" || a[0].OfferID != off.ID {
		t.Fatalf("alerts %+v", a)
	}
}

func enrollPinIsCompareAndSet(t *testing.T) {
	r := newRig(t, nil)
	m1 := pairedNode(t, r, "node-cas")
	// A rotation lands after the re-check, while the certificate is being issued.
	r.exec(`CREATE TRIGGER rotate_late AFTER INSERT ON pki_cert BEGIN UPDATE pki_node_key SET key_id='0123456789abcdef', public='' WHERE node_id='node-cas'; END`)
	ap := approving("policy:again", DefaultTrust())
	_, err := r.redeem(r.offer(OfferOptions{Approver: ap}).Code, "node-cas", newNodeMat(t).withNodeKeyOf(m1))
	if pki.ReasonOf(err) != pki.ReasonNodeKeyChanged {
		t.Fatalf("want node_key_changed, got %v", err)
	}
	if n := r.count(`SELECT count(*) FROM pki_node_key WHERE node_id='node-cas' AND key_id='0123456789abcdef' AND public=''`); n != 1 {
		t.Fatal("the late rotation was overwritten by the old key")
	}
	certs, _ := r.st.PKICertsOfNode(context.Background(), "node-cas")
	if len(certs) != 2 {
		t.Fatalf("certs %+v", certs)
	}
	if n := r.count(`SELECT count(*) FROM pki_denied WHERE node_id='node-cas'`); n != 1 {
		t.Fatalf("the certificate issued against a rotated-away key is usable (denied rows %d)", n)
	}
}

func enrollNodeRemovedDuringApproval(t *testing.T) {
	r := newRig(t, nil)
	m1 := pairedNode(t, r, "node-gone")
	ap := &blockingApprover{entered: make(chan struct{}), release: make(chan struct{}), dec: Decision{Approve: true, Trust: DefaultTrust()}}
	off := r.offer(OfferOptions{Approver: ap})
	done := make(chan error, 1)
	go func() { _, err := r.redeem(off.Code, "node-gone", newNodeMat(t).withNodeKeyOf(m1)); done <- err }()
	waitEntered(t, ap)
	if _, err := r.st.NodeRemove(context.Background(), "node-gone", "cli:test", nil); err != nil {
		t.Fatal(err)
	}
	close(ap.release)
	e659(t, waitErr(t, done), ReasonNodeGone, off.Code)
	if certs, _ := r.st.PKICertsOfNode(context.Background(), "node-gone"); len(certs) != 1 {
		t.Fatalf("a certificate was issued for a removed node: %+v", certs)
	}
	if rows := r.audit("pairing.refuse"); len(rows) != 1 || !strings.Contains(rows[0].Detail, "reason="+ReasonNodeGone) {
		t.Fatalf("audit %+v", rows)
	}
	if len(r.audit("pairing.approve")) != 1 { // only the first pairing
		t.Fatal("the removed node's re-pair was recorded as approved")
	}
}
