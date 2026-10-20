//go:build security

package redteam

// Purpose: isolating regression tests added after the mutation review of the HELD claims (P7-TRUST-18),
// group C: the runner verifier checks that another check also covered, and the TLS user-listener callback.
//   - the evidence revision against the attempt's revision (signed, context matching, only the revision differs);
//   - the decoded evidence against the signed bytes (documentMatches), with a document and an evidence that differ;
//   - the ledger Replayed lookup, with a ledger whose Consume would still succeed;
//   - the TLS VerifyConnection kind and chain checks, called with hand-made connection states so the
//     client-CA pool and the handshake chain building cannot mask them.
// Inputs:  attest.RunnerVerifier over cLedger, pki.CA server configs.
// Outputs: pass (control held) or fail (attack worked).
// Constraints: no network; the ledger is the only fake.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// cReplayLedger reports every signature as already consumed for another attempt, while Consume still succeeds.
type cReplayLedger struct{ *cLedger }

func (cReplayLedger) Replayed(context.Context, string, string, string) (bool, error) {
	return true, nil
}

func TestC26RunnerVerifierChecksStandAlone(t *testing.T) {
	verify := func(l attest.Ledger, doc []byte, ev model.Evidence) error {
		return (&attest.RunnerVerifier{Ledger: func() attest.Ledger { return l }}).VerifyEvidence(doc, ev)
	}
	good := cSign(t, cEvidence(), cHolder)
	if err := verify(cNewLedger(), cDoc(t, good), good); err != nil {
		t.Fatalf("control: honest signed evidence refused: %v", err)
	}
	// freshness is recomputed at read time and not signed: it must not count as a disagreement
	stale := good
	stale.Freshness = model.Freshness{State: "stale"}
	if err := verify(cNewLedger(), cDoc(t, good), stale); err != nil {
		t.Errorf("control: a different freshness state was read as a different document: %v", err)
	}

	// revision: signed by the lease holder, every other field matches, only the revision is not the attempt's
	other := cEvidence()
	other.Revision = strings.Repeat("b", 40)
	other = cSign(t, other, cHolder)
	if err := verify(cNewLedger(), cDoc(t, other), other); attest.RemoteReasonOf(err) != attest.RemoteRevisionMismatch {
		t.Errorf("signed evidence of another revision: reason %q (%v), want %q", attest.RemoteReasonOf(err), err, attest.RemoteRevisionMismatch)
	}

	// document and decoded evidence: the signature covers the document, the checks read another evidence
	for name, edit := range map[string]func(*model.Evidence){
		"result":  func(e *model.Evidence) { e.Result = "fail" },
		"counts":  func(e *model.Evidence) { e.Counts.Pass = 99 },
		"runner":  func(e *model.Evidence) { e.Runner.Isolation = "process" },
		"trigger": func(e *model.Evidence) { e.Trigger = "api" },
	} {
		ev := good
		edit(&ev)
		if err := verify(cNewLedger(), cDoc(t, good), ev); attest.RemoteReasonOf(err) != attest.RemoteSignatureInvalid {
			t.Errorf("%s: a signed document paired with other decoded evidence: reason %q (%v), want %q", name, attest.RemoteReasonOf(err), err, attest.RemoteSignatureInvalid)
		}
	}

	// signing eligibility: the lease holder, signature valid, but the node is not signing eligible
	ineligible := cNewLedger()
	ineligible.nodes[cHolder.KeyID()][0].SigningEligible = false
	if err := verify(ineligible, cDoc(t, good), good); attest.RemoteReasonOf(err) != attest.RemoteNodeNotEligible {
		t.Errorf("a node that is not signing eligible: reason %q (%v), want %q", attest.RemoteReasonOf(err), err, attest.RemoteNodeNotEligible)
	}

	// replay: the lookup refuses before Consume could
	if err := verify(cReplayLedger{cNewLedger()}, cDoc(t, good), good); attest.RemoteReasonOf(err) != attest.RemoteReplay {
		t.Errorf("a signature the ledger reports as replayed: reason %q (%v), want %q", attest.RemoteReasonOf(err), err, attest.RemoteReplay)
	}
}

func TestC23TLSServerCallbackChecksKindAndChainOnItsOwn(t *testing.T) {
	r := cNewRig(t)
	node, user := r.nodeCert("node-1"), r.userCert("user-1")
	leaf, err := r.ca.IssueServer("coord-1", nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	g := pki.NewGuard(r.st)
	_ = g.Refresh(context.Background())
	srv, err := r.ca.TLSServerConfig(pki.KindUser, leaf, g)
	if err != nil {
		t.Fatal(err)
	}
	inter := func(c tls.Certificate) *x509.Certificate {
		x, err := x509.ParseCertificate(c.Certificate[1])
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	nodeInter, userInter := inter(node), inter(user)
	state := func(peer *x509.Certificate, chain ...*x509.Certificate) tls.ConnectionState {
		return tls.ConnectionState{PeerCertificates: []*x509.Certificate{peer}, VerifiedChains: [][]*x509.Certificate{chain}}
	}
	if err := srv.VerifyConnection(state(user.Leaf, user.Leaf, userInter)); err != nil {
		t.Fatalf("control: a user leaf chained to the user intermediate was refused: %v", err)
	}
	for name, c := range map[string]struct {
		cs   tls.ConnectionState
		want string
	}{
		"no peer certificate":               {tls.ConnectionState{}, pki.ReasonNoCert},
		"node leaf under the user chain":    {state(node.Leaf, node.Leaf, userInter), pki.ReasonWrongKind},
		"user leaf under the node chain":    {state(user.Leaf, user.Leaf, nodeInter), pki.ReasonBadChain},
		"user leaf with no verified chains": {tls.ConnectionState{PeerCertificates: []*x509.Certificate{user.Leaf}}, pki.ReasonBadChain},
	} {
		if got := pki.ReasonOf(srv.VerifyConnection(c.cs)); got != c.want {
			t.Errorf("%s: reason %q, want %q", name, got, c.want)
		}
	}
}
