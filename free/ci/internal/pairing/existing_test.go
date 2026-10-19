package pairing

// Purpose: re-pairing of a name that is already registered (review round 1): only the holder of the
// pinned node key of a live node may re-pair, the code is spent either way, the approver's trust fields
// are not applied, and a withdraw or an audit failure during an in-flight approval leaves no usable
// certificate.
// Inputs:  a rig, fake and blocking approvers. Outputs: pass/fail. Constraints: failures are injected with
//          SQLite triggers on a second connection.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

func sshStyleNode(t *testing.T, r *rig, name string) {
	t.Helper()
	m := newNodeMat(t)
	c := draftCapability(name, r.request("AAAAAAAAAA", name, m), r.clock.Now().UTC())
	if _, err := r.reg.Register(context.Background(), c, "cli:test"); err != nil {
		t.Fatal(err)
	}
}

func trustOfNode(t *testing.T, r *rig, name string) string {
	t.Helper()
	n, err := r.reg.Get(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	z := ""
	if n.Capability.Trust.PrivacyZone.Value != nil {
		z = string(*n.Capability.Trust.PrivacyZone.Value)
	}
	var acc, sec any
	if v := n.Capability.Trust.Accepts.Value; v != nil {
		acc = *v
	}
	if v := n.Capability.Trust.SecretClasses.Value; v != nil {
		sec = *v
	}
	return fmt.Sprint(acc, sec, z, n.Capability.Lifecycle.AuthorizedProjects)
}

func enrollExistingNoPin(t *testing.T) {
	r := newRig(t, nil)
	sshStyleNode(t, r, "node-ssh")
	before := trustOfNode(t, r, "node-ssh")
	ap := approving("policy:x", TrustFields{Accepts: []string{"owner"}, SecretClasses: []string{"none"}, PrivacyZone: "local-only", Projects: []string{"proj-9"}})
	off := r.offer(OfferOptions{Approver: ap})
	_, err := r.redeem(off.Code, "node-ssh", newNodeMat(t))
	if pki.ReasonOf(err) != pki.ReasonNodeKeyChanged || !strings.Contains(err.Error(), "E660") {
		t.Fatalf("want E660 node_key_changed, got %v", err)
	}
	if len(ap.calls()) != 0 || r.count(`SELECT count(*) FROM pki_cert`) != 0 || r.count(`SELECT count(*) FROM pki_node_key`) != 0 {
		t.Fatal("an unpinned existing node reached the approver, got a certificate or got a key pinned")
	}
	if trustOfNode(t, r, "node-ssh") != before {
		t.Fatal("the registered trust changed")
	}
	_, err = r.redeem(off.Code, "node-ssh", newNodeMat(t)) // the code was consumed by the attempt
	e659(t, err, ReasonReused, off.Code)
}

func enrollExistingSameKey(t *testing.T) {
	r := newRig(t, nil)
	first := approving("policy:first", TrustFields{Accepts: []string{"owner"}, SecretClasses: []string{"none"}, PrivacyZone: "local-only", Projects: []string{"proj-1"}})
	m1 := newNodeMat(t)
	if _, err := r.redeem(r.offer(OfferOptions{Approver: first}).Code, "node-re", m1); err != nil {
		t.Fatal(err)
	}
	kept := trustOfNode(t, r, "node-re")
	second := approving("policy:second", TrustFields{Accepts: []string{"internal"}, SecretClasses: []string{"none"}, PrivacyZone: "hosted-allowed", Projects: []string{"proj-2"}})
	off := r.offer(OfferOptions{Approver: second})
	m2 := newNodeMat(t) // a new TLS key and CSR, the same node key
	m2.pub = m1.pub
	if _, err := r.redeem(off.Code, "node-re", m2); err != nil {
		t.Fatalf("re-pair with the pinned key: %v", err)
	}
	if got := first.calls(); len(got) != 1 || got[0].Existing {
		t.Fatalf("first pairing: %+v", got)
	}
	if got := second.calls(); len(got) != 1 || !got[0].Existing || got[0].CSRFingerprint != m2.csrFP {
		t.Fatalf("re-pair: %+v", got)
	}
	if trustOfNode(t, r, "node-re") != kept {
		t.Fatalf("trust changed on re-pair: %s -> %s", kept, trustOfNode(t, r, "node-re"))
	}
	rows := r.audit("pairing.approve")
	if len(rows) != 2 || !strings.Contains(rows[1].Detail, "trust=kept") || strings.Contains(rows[1].Detail, "accepts=") || strings.Contains(rows[1].Detail, "proj-2") {
		t.Fatalf("audit %+v", rows)
	}
	if !strings.Contains(rows[0].Detail, "accepts=owner") || strings.Contains(rows[0].Detail, "trust=kept") {
		t.Fatalf("first audit %+v", rows[0])
	}
	if certs, _ := r.st.PKICertsOfNode(context.Background(), "node-re"); len(certs) != 2 {
		t.Fatalf("certs %+v", certs)
	}
}

func enrollExistingOtherKey(t *testing.T) {
	r := newRig(t, nil)
	if _, err := r.redeem(r.offer(OfferOptions{Approver: approving("policy:first", DefaultTrust())}).Code, "node-re", newNodeMat(t)); err != nil {
		t.Fatal(err)
	}
	ap := approving("policy:thief", DefaultTrust())
	off := r.offer(OfferOptions{Approver: ap})
	_, err := r.redeem(off.Code, "node-re", newNodeMat(t))
	if pki.ReasonOf(err) != pki.ReasonNodeKeyChanged {
		t.Fatalf("want node_key_changed, got %v", err)
	}
	if len(ap.calls()) != 0 || r.count(`SELECT count(*) FROM pki_cert`) != 1 {
		t.Fatal("another key reached the approver or got a certificate")
	}
	if _, err = r.redeem(off.Code, "node-re", newNodeMat(t)); ReasonOf(err) != ReasonReused {
		t.Fatalf("the code must be spent: %v", err)
	}
}

func enrollExistingRevoked(t *testing.T) {
	r := newRig(t, nil)
	m := newNodeMat(t)
	if _, err := r.redeem(r.offer(OfferOptions{Approver: approving("policy:first", DefaultTrust())}).Code, "node-rev", m); err != nil {
		t.Fatal(err)
	}
	if _, err := r.st.NodeRevoke(context.Background(), "node-rev", "cli:test", nil); err != nil {
		t.Fatal(err)
	}
	ap := approving("policy:again", DefaultTrust())
	_, err := r.redeem(r.offer(OfferOptions{Approver: ap}).Code, "node-rev", m) // even with the pinned key
	if pki.ReasonOf(err) != pki.ReasonRevoked || len(ap.calls()) != 0 || r.count(`SELECT count(*) FROM pki_cert`) != 1 {
		t.Fatalf("a revoked node re-paired: %v", err)
	}
}

func enrollWithdrawDuringApproval(t *testing.T) {
	r := newRig(t, nil)
	ap := &blockingApprover{entered: make(chan struct{}), release: make(chan struct{}), dec: Decision{Approve: true, Trust: DefaultTrust()}}
	off := r.offer(OfferOptions{Approver: ap})
	done := make(chan error, 1)
	go func() { _, err := r.redeem(off.Code, "node-wd", newNodeMat(t)); done <- err }()
	waitEntered(t, ap)
	if err := r.svc.Withdraw(context.Background(), off.Code); err != nil {
		t.Fatal(err)
	}
	close(ap.release) // the approver had already approved
	err := waitErr(t, done)
	e659(t, err, ReasonWithdrawn, off.Code)
	certs, _ := r.st.PKICertsOfNode(context.Background(), "node-wd")
	if len(certs) != 1 {
		t.Fatalf("certs %+v", certs)
	}
	if denied, _ := r.st.PKIIsDenied(context.Background(), certs[0].Serial); !denied {
		t.Fatal("the certificate issued under a withdrawn offer is usable")
	}
	if len(r.audit("pairing.approve")) != 0 || len(r.alerts.all()) != 0 {
		t.Fatalf("audit %+v alerts %+v", r.audit(""), r.alerts.all())
	}
}

func waitEntered(t *testing.T, ap *blockingApprover) {
	t.Helper()
	select {
	case <-ap.entered:
	case <-time.After(60 * time.Second):
		t.Fatal("the redemption never reached the approver")
	}
}

func waitErr(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(60 * time.Second):
		t.Fatal("the redemption never finished")
	}
	return nil
}

func enrollApproveAuditFailsClosed(t *testing.T) {
	r := newRig(t, nil)
	off := r.offer(OfferOptions{Approver: approving("policy:a", DefaultTrust())})
	r.exec(`CREATE TRIGGER no_approve_audit BEFORE INSERT ON audit WHEN NEW.action='pairing.approve' BEGIN SELECT RAISE(ABORT,'audit refused'); END`)
	resp, err := r.redeem(off.Code, "node-au", newNodeMat(t))
	if err == nil || strings.Contains(err.Error(), "E659") || !strings.Contains(err.Error(), "could not be audited") || len(resp.Cert) != 0 {
		t.Fatalf("err %v resp %+v", err, resp)
	}
	certs, _ := r.st.PKICertsOfNode(context.Background(), "node-au")
	if len(certs) != 1 {
		t.Fatalf("certs %+v", certs)
	}
	if denied, _ := r.st.PKIIsDenied(context.Background(), certs[0].Serial); !denied {
		t.Fatal("a certificate whose approval could not be audited is usable")
	}
	if r.svc.redeemedOffer(context.Background(), off.ID) {
		t.Fatal("the offer counts as redeemed")
	}
}

func enrollReuseRevokeFailureIsInternal(t *testing.T) {
	r := newRig(t, nil)
	off := r.offer(OfferOptions{Approver: approving("policy:a", DefaultTrust())})
	first, err := r.redeem(off.Code, "node-rv", newNodeMat(t))
	if err != nil {
		t.Fatal(err)
	}
	r.exec(`CREATE TRIGGER no_deny BEFORE INSERT ON pki_denied BEGIN SELECT RAISE(ABORT,'deny refused'); END`)
	_, err = r.redeem(off.Code, "node-thief", newNodeMat(t))
	if err == nil || ReasonOf(err) != "" || strings.Contains(err.Error(), "was revoked") || !strings.Contains(err.Error(), "could not be revoked") {
		t.Fatalf("want a plain internal error saying the revoke failed, got %v", err)
	}
	if denied, _ := r.st.PKIIsDenied(context.Background(), hexSerial(t, first.Cert)); denied {
		t.Fatal("the deny trigger did not hold")
	}
	if len(r.alerts.all()) != 0 {
		t.Fatalf("an alert claimed a revoke that did not happen: %+v", r.alerts.all())
	}
}
