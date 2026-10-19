package pairing

// Purpose: TestEnroll, the loopback evidence of the enrollment flow of P7-NODE-07: one t.Run per
// acceptance line. Real TLS 1.3 listeners on 127.0.0.1:0, a real pki.CA over a real SQLite store.
// Inputs:  none. Outputs: pass/fail. Constraints: no sleeps over the 1.5 s bound; the rate-limit wait of
//          join advances the fake clock.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

func TestEnroll(t *testing.T) {
	t.Run("end to end on loopback: join with --expect-fingerprint enrolls and the listener closes after redemption", enrollEndToEnd)
	t.Run("the listener closes after 10 minutes without a redemption", enrollExpiryCloses)
	t.Run("a MITM proxy presenting its own root is detected at the fingerprint step and nothing is sent", enrollMITMOwnRoot)
	t.Run("a proxy with the real root and intermediate but its own leaf fails chain verification", enrollMITMForgedLeaf)
	t.Run("code reuse, expiry, lockout and withdrawal fail with E659 and distinct reasons", enrollReasons)
	t.Run("more than 5 wrong attempts lock the offer, 4 do not", enrollLockBoundary)
	t.Run("a second redemption revokes the first certificate, writes an audit row and raises the alert", enrollReuseRevokes)
	t.Run("a code presented again while the first approval is in flight revokes the certificate issued for it", enrollReuseDuringApproval)
	t.Run("an existing node with no pinned key is refused before the approver and the code is spent", enrollExistingNoPin)
	t.Run("an existing node re-pairs with its pinned key and keeps its trust", enrollExistingSameKey)
	t.Run("an existing node with another key is refused before the approver", enrollExistingOtherKey)
	t.Run("a revoked node cannot re-pair", enrollExistingRevoked)
	t.Run("a withdraw during an in-flight approval ends E659 withdrawn with no usable certificate", enrollWithdrawDuringApproval)
	t.Run("an approval that cannot be audited fails closed and revokes the certificate", enrollApproveAuditFailsClosed)
	t.Run("a reuse whose revoke fails is an internal error, not the revoked message", enrollReuseRevokeFailureIsInternal)
	t.Run("takeover of a registered node with only its public node key is refused node_key_proof and the code is not consumed", enrollTakeoverWithPublicKeyOnly)
	t.Run("a signature over any other transcript is refused node_key_proof", enrollWrongTranscript)
	t.Run("the burst listener refuses an already registered node name, audits and alerts", enrollBurstRefusesExistingNode)
	t.Run("a refusal after the claim writes a pairing.refuse audit row and a takeover alert", enrollRefusalsAreAuditedAndAlerted)
	t.Run("a pending pin survives a denied or refused redemption and is finalized by an approved one", enrollPendingPinSurvivesDenial)
	t.Run("an offer owned by another process is refused without being consumed or counted", enrollForeignOfferNotBurned)
	t.Run("join does not follow redirects: the code and CSR never reach the target", enrollJoinDoesNotFollowRedirects)
	t.Run("server kind: an agent client rejects node-kind and user-kind leaves chained to the same root", enrollServerKinds)
	t.Run("clock skew over 120 s is E660 on the server and does not consume the code", enrollSkewServer)
	t.Run("clock skew over 120 s is E660 on the node before any CSR is sent", enrollSkewClient)
	t.Run("a malformed request does not consume the code", enrollMalformedKeepsCode)
	t.Run("the enrollment route: POST only, caps, strict JSON, rate limit, no echo of secrets, graceful close", enrollHandlerStack)
}

func enrollEndToEnd(t *testing.T) {
	r := newRig(t, nil)
	ap := approving("policy:e2e", DefaultTrust())
	w, wait := r.window(OfferOptions{Approver: ap, Label: "lab box"})
	if !isOpen(w.Addr.String()) {
		t.Fatal("the window did not bind")
	}
	res, err := r.join(w.URL, w.Offer.Code, w.Offer.RootFingerprint, "Node-A")
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if res.NodeID != "node-a" || res.RootFingerprint != w.Offer.RootFingerprint || res.AgentURL != "https://agent.test:3865" {
		t.Fatalf("result %+v", res)
	}
	if wr := r.waitFor(wait, 3*time.Second); !wr.redeemed || wr.err != nil {
		t.Fatalf("Wait = %+v", wr)
	}
	eventually(t, time.Second, "the listener closes after redemption", func() bool { return !isOpen(w.Addr.String()) })
	got := ap.calls()
	if len(got) != 1 || got[0].CSRFingerprint != res.CSRFingerprint || got[0].Label != "lab box" || got[0].NodeID != "node-a" {
		t.Fatalf("approver saw %+v", got)
	}
	if got[0].NodeKeyID == "" || got[0].SourceAddr != "127.0.0.1" || got[0].OS != "linux" {
		t.Fatalf("approval request %+v", got[0])
	}
	n, err := r.reg.Get(context.Background(), "node-a")
	if err != nil || n.Record.State == "revoked" {
		t.Fatalf("registry: %v %+v", err, n.Record)
	}
	certs, _ := r.st.PKICertsOfNode(context.Background(), "node-a")
	if len(certs) != 1 || certs[0].OfferID != w.Offer.ID || certs[0].Kind != "node" {
		t.Fatalf("certs %+v", certs)
	}
	if id, kind := pki.IdentityOf(mustParse(t, res.CertDER)); id != "node-a" || kind != pki.KindNode {
		t.Fatalf("identity %q %q", id, kind)
	}
	r.noCodeInAudit(w.Offer.Code)
}

func mustParse(t testing.TB, der []byte) *x509.Certificate {
	t.Helper()
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func enrollExpiryCloses(t *testing.T) {
	r := newRig(t, nil)
	w, wait := r.window(OfferOptions{Approver: approving("policy:x", DefaultTrust())})
	r.clock.Advance(OfferTTL - time.Second)
	time.Sleep(150 * time.Millisecond)
	select {
	case <-wait:
		t.Fatal("the window ended before the 10 minutes")
	default:
	}
	if !isOpen(w.Addr.String()) {
		t.Fatal("the listener closed before the 10 minutes")
	}
	r.clock.Advance(2 * time.Second)
	if wr := r.waitFor(wait, time.Second); wr.redeemed || wr.err != nil {
		t.Fatalf("Wait = %+v", wr)
	}
	eventually(t, time.Second, "the listener closes after expiry", func() bool { return !isOpen(w.Addr.String()) })
	if _, err := r.redeem(w.Offer.Code, "late", newNodeMat(t)); ReasonOf(err) != ReasonExpired {
		t.Fatalf("late redeem: %v", err)
	}
}

func enrollMITMOwnRoot(t *testing.T) {
	r := newRig(t, nil)
	ap := approving("policy:mitm", DefaultTrust())
	off := r.offer(OfferOptions{Approver: ap})
	real := r.serveReal()
	attacker := newRig(t, nil)
	leaf, err := attacker.ca.IssueServer("local", nil, loopback, 0)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, hits := proxy(t, leaf)
	if attacker.ca.RootFingerprint() == off.RootFingerprint {
		t.Fatal("two CAs share a root")
	}
	// The operator compares the real root fingerprint (printed by `nodes pair`) with --expect-fingerprint.
	r.clock.Advance(2 * time.Second)
	out := &syncBuf{}
	opts, _ := r.joinOpts(proxyURL, off.Code, off.RootFingerprint, "victim", out)
	codeAsked := false
	opts.Code = func() (string, error) { codeAsked = true; return off.Code, nil }
	_, err = runJoin(opts)
	if pki.ReasonOf(err) != pki.ReasonFingerprintMismatch {
		t.Fatalf("want E660 fingerprint_mismatch, got %v", err)
	}
	if !strings.Contains(err.Error(), "E660") {
		t.Fatalf("not an E660 error: %v", err)
	}
	// A scripted operator who compares by eye also refuses.
	opts.ExpectFingerprint = ""
	opts.Confirm = func(shown string) (bool, error) { return shown == off.RootFingerprint, nil }
	if _, err = runJoin(opts); pki.ReasonOf(err) != pki.ReasonFingerprintUnconfirmed {
		t.Fatalf("scripted operator: %v", err)
	}
	if hits.Load() != 0 || codeAsked || strings.Contains(out.String(), "CSR key fingerprint") {
		t.Fatalf("something was sent: proxy hits %d, code asked %v, out %q", hits.Load(), codeAsked, out.String())
	}
	if real.Cnt.hellos.Load() != 0 || real.Cnt.redeem.Load() != 0 || len(ap.calls()) != 0 {
		t.Fatal("the real listener or approver saw a request")
	}
	if live, _ := r.st.PKIOffersLive(context.Background(), false); len(live) != 1 || live[0].Attempts != 0 {
		t.Fatalf("offer touched: %+v", live)
	}
	if n := r.count(`SELECT count(*) FROM pki_cert`); n != 0 {
		t.Fatalf("%d certificates issued", n)
	}
}

func enrollMITMForgedLeaf(t *testing.T) {
	r := newRig(t, nil)
	ap := approving("policy:mitm", DefaultTrust())
	off := r.offer(OfferOptions{Approver: ap})
	real := r.serveReal()
	good, err := r.ca.IssueServer("local", nil, loopback, 0)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, hits := proxy(t, forgedLeaf(t, good))
	r.clock.Advance(2 * time.Second)
	opts, _ := r.joinOpts(proxyURL, off.Code, off.RootFingerprint, "victim", &syncBuf{})
	codeAsked := false
	opts.Code = func() (string, error) { codeAsked = true; return off.Code, nil }
	_, err = runJoin(opts)
	if err == nil {
		t.Fatal("join accepted a leaf the real intermediate never signed")
	}
	var ue x509.UnknownAuthorityError
	if !errors.As(err, &ue) {
		t.Fatalf("not a chain failure (want unknown authority): %v", err)
	}
	if hits.Load() != 0 || codeAsked || real.Cnt.hellos.Load() != 0 || len(ap.calls()) != 0 {
		t.Fatalf("something was sent: hits %d asked %v", hits.Load(), codeAsked)
	}
}

func enrollKindsOf(t *testing.T, r *rig) map[string]tls.Certificate {
	ctx := context.Background()
	coord, err := r.ca.IssueServer("local", nil, loopback, 0)
	if err != nil {
		t.Fatal(err)
	}
	m := newNodeMat(t)
	iss, err := r.ca.IssueNode(ctx, pki.NodeRequest{CSR: m.csr, NodeID: "kind-node"})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := r.ca.IssueUser(ctx, "alice", 0)
	if err != nil {
		t.Fatal(err)
	}
	user, err := tls.X509KeyPair(bundle.CertPEM, bundle.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]tls.Certificate{
		"coordinator": coord,
		"node":        {Certificate: [][]byte{iss.CertDER, iss.Chain[1], iss.RootDER}, PrivateKey: m.key},
		"user":        user,
	}
}

func enrollServerKinds(t *testing.T) {
	r := newRig(t, nil)
	other := newRig(t, nil)
	kinds := enrollKindsOf(t, r)
	foreign, err := other.ca.IssueServer("local", nil, loopback, 0)
	if err != nil {
		t.Fatal(err)
	}
	table := []struct {
		name string
		cert tls.Certificate
		ok   bool
	}{
		{"coordinator leaf of the pinned root", kinds["coordinator"], true},
		{"node-kind leaf of the same root", kinds["node"], false},
		{"user-kind leaf of the same root", kinds["user"], false},
		{"coordinator leaf of another root", foreign, false},
	}
	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			addr := okServer(t, &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{tc.cert}})
			err := dialOK(addr, r.clientCfg(nil))
			if tc.ok && err != nil {
				t.Fatalf("a good server was refused: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("the agent client accepted a server it must refuse")
			}
		})
	}
}

func hexSerial(t testing.TB, der []byte) string {
	return hex.EncodeToString(mustParse(t, der).SerialNumber.Bytes())
}
