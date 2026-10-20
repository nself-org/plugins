package cloudvm_test

// Purpose: TestEnrollWhileIntents*, TestCloudVMFullCycle*: enrollment of burst VMs. One listener for all
// live intents (open with the first offer, closed when the last offer is redeemed, withdrawn or
// expired); a code of a terminal intent is refused; provision -> join -> teardown ends with the
// certificate failing VerifyConnection and no labelled server left.
// Inputs:  the rig (real store, CA, pairing, burst engine; fake nself and fake cloud). Outputs: pass/fail.
// Constraints: the clock is stepped, never slept on; loopback only.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/providers/ephemeral/cloudvm"
)

func reasonOfJoin(err error) string {
	var ce enroll.CodedError
	if errors.As(err, &ce) {
		return ce.ErrReason()
	}
	return ""
}

func TestEnrollWhileIntentsOneListenerForManyIntents(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.serve()
	if isOpen(r.enrollAdr) {
		t.Fatal("the listener is open with no offer")
	}
	a, b := r.provision("job-1"), r.provision("job-2")
	eventually(t, 2*time.Second, "the first offer opens the listener", func() bool { return isOpen(r.enrollAdr) })
	codeA, fpA := r.userDataOf(a.Token)
	codeB, fpB := r.userDataOf(b.Token)
	if _, err := r.join(codeA, fpA, cloudvm.NodeName(a.Token)); err != nil {
		t.Fatalf("join A: %v", err)
	}
	if !isOpen(r.enrollAdr) {
		t.Fatal("the listener closed while intent B still has a live offer")
	}
	if _, err := r.join(codeB, fpB, cloudvm.NodeName(b.Token)); err != nil {
		t.Fatalf("join B: %v", err)
	}
	eventually(t, 2*time.Second, "the last redemption closes the listener", func() bool { return !isOpen(r.enrollAdr) })
	for _, tok := range []string{a.Token, b.Token} {
		if in := r.intent(tok); in.NodeID != cloudvm.NodeName(tok) {
			t.Fatalf("intent %s is bound to node %q", tok, in.NodeID)
		}
	}

	// expiry closes it too: a new offer opens the listener, ten minutes later it is closed
	c := r.provision("job-3")
	eventually(t, 2*time.Second, "a new offer reopens the listener", func() bool { return isOpen(r.enrollAdr) })
	r.clk.Advance(pairing.OfferTTL + time.Minute)
	eventually(t, 2*time.Second, "an expired offer closes the listener", func() bool { return !isOpen(r.enrollAdr) })
	codeC, fpC := r.userDataOf(c.Token)
	if _, err := r.join(codeC, fpC, cloudvm.NodeName(c.Token)); err == nil {
		t.Fatal("an expired code enrolled a node")
	}
	_ = ctx
}

func TestEnrollWhileIntentsRefusesTheCodeOfATerminalIntent(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.serve()
	dead, live := r.provision("job-1"), r.provision("job-2")
	eventually(t, 2*time.Second, "the listener opens", func() bool { return isOpen(r.enrollAdr) })
	code, fp := r.userDataOf(dead.Token)
	before := r.auditCount("pairing.withdraw")
	if err := r.eng.Teardown(ctx, dead.Token, "job done"); err != nil {
		t.Fatal(err)
	}
	if r.auditCount("pairing.withdraw") != before+1 {
		t.Fatal("the withdrawal of the terminal intent's offer was not audited")
	}
	if !isOpen(r.enrollAdr) {
		t.Fatal("the listener must stay open for the live intent")
	}
	_, err := r.join(code, fp, cloudvm.NodeName(dead.Token))
	if got := reasonOfJoin(err); err == nil || got != pairing.ReasonNoIntent && got != pairing.ReasonWithdrawn {
		t.Fatalf("want a refusal (no live intent or withdrawn), got %v", err)
	}
	if _, gerr := r.reg.Get(ctx, cloudvm.NodeName(dead.Token)); gerr == nil {
		t.Fatal("a node of the terminal intent is registered")
	}
	_ = live
}

func TestEnrollWhileIntentsRefusesACodeWithoutALiveIntent(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.serve()
	// a machine whose intent row never existed (or is gone): the engine is not involved
	tok := "01J0000000000000000000NOINT"
	if _, err := r.prov.Provision(ctx, intentOf(tok)); err != nil {
		t.Fatal(err)
	}
	eventually(t, 2*time.Second, "the offer opens the listener", func() bool { return isOpen(r.enrollAdr) })
	code, fp := r.userDataOf(tok)
	if _, err := r.join(code, fp, cloudvm.NodeName(tok)); err == nil {
		t.Fatal("a code without a live intent enrolled a node")
	}
	if _, err := r.reg.Get(ctx, cloudvm.NodeName(tok)); err == nil {
		t.Fatal("the node is registered")
	}
}

func TestEnrollWhileIntentsDeniesAnotherNodeName(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.serve()
	a := r.provision("job-1")
	eventually(t, 2*time.Second, "the listener opens", func() bool { return isOpen(r.enrollAdr) })
	code, fp := r.userDataOf(a.Token)
	before := r.auditCount("pairing.deny")
	if _, err := r.join(code, fp, "some-other-node"); err == nil || reasonOfJoin(err) != pairing.ReasonDenied {
		t.Fatalf("a code holder must not pick its node name: %v", err)
	}
	if r.auditCount("pairing.deny") != before+1 {
		t.Fatal("the denial was not audited")
	}
	if _, err := r.reg.Get(ctx, "some-other-node"); err == nil {
		t.Fatal("the node was registered")
	}
	if in := r.intent(a.Token); in.NodeID != "" {
		t.Fatalf("the intent was bound to %q", in.NodeID)
	}
}

// verify runs the coordinator's TLS verification of a node certificate (the guard reads the deny list).
func (r *rig) verify(res enroll.JoinResult) error {
	r.t.Helper()
	leaf, err := r.ca.IssueServer("local", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	g := pki.NewGuard(r.st)
	if err = g.Refresh(context.Background()); err != nil {
		r.t.Fatal(err)
	}
	cfg, err := r.ca.TLSServerConfig(pki.KindNode, leaf, g)
	if err != nil {
		r.t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(res.CertDER)
	if err != nil {
		r.t.Fatal(err)
	}
	chain := []*x509.Certificate{cert}
	for _, der := range res.Chain {
		c, perr := x509.ParseCertificate(der)
		if perr != nil {
			r.t.Fatal(perr)
		}
		chain = append(chain, c)
	}
	return cfg.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{chain}})
}

func TestCloudVMFullCycleProvisionJoinJobTeardown(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.serve()
	res := r.provision("job-1")
	name := cloudvm.NodeName(res.Token)
	if _, err := r.reg.Get(ctx, name); err == nil {
		t.Fatal("the node exists before the code was redeemed: a lease could precede enrollment")
	}
	eventually(t, 2*time.Second, "the listener opens", func() bool { return isOpen(r.enrollAdr) })
	code, fp := r.userDataOf(res.Token)
	joined, err := r.join(code, fp, name)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.verify(joined); err != nil {
		t.Fatalf("a joined node's certificate must verify: %v", err)
	}
	if in := r.intent(res.Token); in.NodeID != name || in.State != "live" {
		t.Fatalf("intent %+v", in)
	}
	n, err := r.reg.Get(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	c := n.Capability
	if !slices.Contains(c.Identity.Labels, "nself.burst.intent="+res.Token) || c.Identity.Provider != "ephemeral" {
		t.Fatalf("labels %v provider %q", c.Identity.Labels, c.Identity.Provider)
	}
	if *c.Location.Kind.Value != "hosted" || *c.Economics.Class.Value != "ephemeral" || *c.Trust.Isolation.Value != "ephemeral-vm" || *c.Trust.PrivacyZone.Value != "provider-allowlist" {
		t.Fatalf("kind %v class %v isolation %v zone %v", *c.Location.Kind.Value, *c.Economics.Class.Value, *c.Trust.Isolation.Value, *c.Trust.PrivacyZone.Value)
	}
	if got := *c.Trust.SecretClasses.Value; len(got) != 1 || got[0] != "none" {
		t.Fatalf("secret classes %v", got)
	}
	var accepts []string
	for _, a := range *c.Trust.Accepts.Value {
		accepts = append(accepts, string(a))
	}
	if !slices.Equal(accepts, []string{"owner", "collaborator", "internal"}) {
		t.Fatalf("accepts %v", accepts)
	}

	// the job ran (one job per node); the end of it tears the machine down and revokes the node
	if err = r.eng.Teardown(ctx, res.Token, "job done"); err != nil {
		t.Fatal(err)
	}
	if left := r.labelled(); len(left) != 0 {
		t.Fatalf("labelled servers left: %+v", left)
	}
	err = r.verify(joined)
	var pe *pki.Error
	if !errors.As(err, &pe) || pe.Reason != pki.ReasonRevoked {
		t.Fatalf("after teardown the certificate must fail VerifyConnection as revoked, got %v", err)
	}
	if !slices.Contains(r.denied, name) || r.intent(res.Token).State != "terminal" {
		t.Fatalf("denied %v intent %+v", r.denied, r.intent(res.Token))
	}
	if !strings.Contains(strings.Join(slicesOf(r.auditActions()), ","), "pairing.approve") {
		t.Fatal("the approval was not audited")
	}
}

func (r *rig) auditActions() []string {
	es, err := r.st.AuditEntries(context.Background(), 0, 100000)
	if err != nil {
		r.t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Action)
	}
	return out
}

func slicesOf(s []string) []string { return s }
