package pairing

// Purpose: proof of possession of the node key, the refusals that follow a claim (audit row and alert),
// the read-only vetting of a pending pin, foreign offers, and redirects (Opus review round 2). The first
// tests are the Opus probes turned into permanent tests, with the expectation inverted.
// Inputs:  a rig, forged requests. Outputs: pass/fail. Constraints: Redeem and RedeemIntent directly.

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

func (r *rig) untouched(offer OfferResult) {
	r.tb.Helper()
	live, _ := r.st.PKIOffersLive(context.Background(), false)
	for _, o := range live {
		if o.ID == offer.ID {
			if o.Attempts != 0 || o.RedeemedAt != 0 || o.Locked {
				r.tb.Fatalf("the offer was touched: %+v", o)
			}
			return
		}
	}
	r.tb.Fatal("the offer is no longer live")
}

// TestEnrollTakeoverWithPublicKeyOnly is the Opus probe TestOpusProbeTakeoverWithPublicKeyOnly: the node
// key is public by design, so naming it must not be enough to take over a registered node.
func enrollTakeoverWithPublicKeyOnly(t *testing.T) {
	r := newRig(t, nil)
	victim := newNodeMat(t)
	trusted := approving("admin", TrustFields{Accepts: []string{"owner"}, SecretClasses: []string{"deploy"}, PrivacyZone: "local-only", Projects: []string{"proj-1"}})
	if _, err := r.redeem(r.offer(OfferOptions{Approver: trusted}).Code, "build-mac", victim); err != nil {
		t.Fatal(err)
	}
	kept := trustOfNode(t, r, "build-mac")
	policy := approving("policy:burst", DefaultTrust())
	off := r.offer(OfferOptions{Approver: policy, Intent: "burst-42", CertTTL: 3600e9})
	atk := newNodeMat(t)
	atk.pub = victim.pub // the public half only; atk.priv is the attacker's own key
	for name, call := range map[string]func() error{
		"burst listener": func() error {
			_, err := r.svc.RedeemIntent(context.Background(), r.request(off.Code, "build-mac", atk), "203.0.113.9:4444")
			return err
		},
		"foreground": func() error { _, err := r.redeem(off.Code, "build-mac", atk); return err },
		"no signature at all": func() error {
			q := r.request(off.Code, "build-mac", atk)
			q.NodeKeySig = nil
			_, err := r.svc.Redeem(context.Background(), q, "203.0.113.9:4444")
			return err
		},
		"a new name with a key it does not hold": func() error { _, err := r.redeem(off.Code, "fresh-name", atk); return err },
	} {
		err := call()
		if pki.ReasonOf(err) != pki.ReasonNodeKeyProof || !strings.Contains(err.Error(), "E660") {
			t.Fatalf("%s: want E660 node_key_proof, got %v", name, err)
		}
	}
	if len(policy.calls()) != 0 || r.count(`SELECT count(*) FROM pki_cert`) != 1 || trustOfNode(t, r, "build-mac") != kept {
		t.Fatal("a takeover reached the approver, got a certificate or changed the trust")
	}
	r.untouched(off) // the code was not consumed, not even counted as a wrong attempt
	if _, err := r.redeem(off.Code, "honest-node", newNodeMat(t)); err != nil {
		t.Fatalf("the legitimate holder of the code was locked out: %v", err)
	}
}

func enrollWrongTranscript(t *testing.T) {
	r := newRig(t, nil)
	ap := approving("policy:p", DefaultTrust())
	off := r.offer(OfferOptions{Approver: ap})
	m := newNodeMat(t)
	other := newNodeMat(t)
	now := r.clock.Now().Unix()
	id := pki.NodeKeyID(m.pub)
	sign := func(msg []byte) []byte { return ed25519.Sign(m.priv, msg) }
	root := r.ca.RootFingerprint()
	good := enroll.Transcript("local", root, "node-t", now, m.csrFP, id)
	flipped := append([]byte(nil), sign(good)...)
	flipped[10] ^= 1
	for name, sig := range map[string][]byte{
		"another coordinator id":  sign(enroll.Transcript("elsewhere", root, "node-t", now, m.csrFP, id)),
		"another root":            sign(enroll.Transcript("local", "SHA256:other", "node-t", now, m.csrFP, id)),
		"another node name":       sign(enroll.Transcript("local", root, "node-u", now, m.csrFP, id)),
		"another CSR fingerprint": sign(enroll.Transcript("local", root, "node-t", now, other.csrFP, id)),
		"another client time":     sign(enroll.Transcript("local", root, "node-t", now+1, m.csrFP, id)),
		"another node key id":     sign(enroll.Transcript("local", root, "node-t", now, m.csrFP, pki.NodeKeyID(other.pub))),
		"another domain":          sign([]byte(strings.Replace(string(good), enroll.Domain, "nself-ci-enroll-v2", 1))),
		"the empty message":       sign(nil),
		"a flipped bit":           flipped,
		"a truncated signature":   sign(good)[:63],
		"an empty signature":      nil,
		"another key's signature": ed25519.Sign(other.priv, good),
	} {
		q := r.request(off.Code, "node-t", m)
		q.NodeKeySig = sig
		_, err := r.svc.Redeem(context.Background(), q, "127.0.0.1:1")
		if pki.ReasonOf(err) != pki.ReasonNodeKeyProof {
			t.Errorf("%s: want node_key_proof, got %v", name, err)
		}
	}
	if len(ap.calls()) != 0 {
		t.Fatal("a bad proof reached the approver")
	}
	r.untouched(off)
	q := r.request(off.Code, "node-t", m)
	q.NodeKeySig = sign(good)
	if _, err := r.svc.Redeem(context.Background(), q, "127.0.0.1:1"); err != nil {
		t.Fatalf("the right transcript: %v", err)
	}
	// A signature made for another request is not replayable: it names another CSR.
	q2 := r.request(r.offer(OfferOptions{Approver: ap}).Code, "node-t", other.withNodeKeyOf(m))
	q2.NodeKeySig = sign(good)
	if _, err := r.svc.Redeem(context.Background(), q2, "127.0.0.1:1"); pki.ReasonOf(err) != pki.ReasonNodeKeyProof {
		t.Fatalf("replayed signature: %v", err)
	}
}

func enrollBurstRefusesExistingNode(t *testing.T) {
	r := newRig(t, nil)
	victim := newNodeMat(t)
	if _, err := r.redeem(r.offer(OfferOptions{Approver: approving("admin", DefaultTrust())}).Code, "build-mac", victim); err != nil {
		t.Fatal(err)
	}
	ap := approving("policy:burst", DefaultTrust())
	off := r.offer(OfferOptions{Approver: ap, Intent: "burst-1"})
	// Even the real node, with the right key and a valid proof, does not re-pair through the burst listener.
	_, err := r.svc.RedeemIntent(context.Background(), r.request(off.Code, "build-mac", newNodeMat(t).withNodeKeyOf(victim)), "203.0.113.9:1")
	e659(t, err, ReasonExistingNode, off.Code)
	if len(ap.calls()) != 0 || r.count(`SELECT count(*) FROM pki_cert`) != 1 {
		t.Fatal("the approver was called or a certificate issued")
	}
	rows := r.audit("pairing.refuse")
	if len(rows) != 1 || rows[0].Target != off.ID || !strings.Contains(rows[0].Detail, "node=build-mac") || !strings.Contains(rows[0].Detail, "reason=existing_node") {
		t.Fatalf("audit %+v", rows)
	}
	al := r.alerts.all()
	if len(al) != 1 || al[0].Kind != "pairing.takeover_attempt" || al[0].NodeID != "build-mac" || al[0].OfferID != off.ID {
		t.Fatalf("alerts %+v", al)
	}
	r.noCodeInAudit(off.Code)
	_, err = r.svc.RedeemIntent(context.Background(), r.request(off.Code, "new-burst", newNodeMat(t)), "203.0.113.9:1")
	e659(t, err, ReasonReused, off.Code) // the code was consumed by the attempt
}

func enrollRefusalsAreAuditedAndAlerted(t *testing.T) {
	type tc struct {
		name, reason string
		setup        func(r *rig, m nodeMat)
		req          func(r *rig, code string, m nodeMat) enroll.Request
		alert        bool
	}
	pinned := func(r *rig, m nodeMat) {
		if _, err := r.redeem(r.offer(OfferOptions{Approver: approving("admin", DefaultTrust())}).Code, "n1", m); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []tc{
		{"another key for a registered name", pki.ReasonNodeKeyChanged, pinned, func(r *rig, code string, m nodeMat) enroll.Request { return r.request(code, "n1", newNodeMat(t)) }, true},
		{"a registered name with no pin", pki.ReasonNodeKeyChanged, func(r *rig, m nodeMat) { sshStyleNode(t, r, "n1") },
			func(r *rig, code string, m nodeMat) enroll.Request { return r.request(code, "n1", newNodeMat(t)) }, true},
		{"a revoked name", pki.ReasonRevoked, func(r *rig, m nodeMat) {
			pinned(r, m)
			if _, err := r.st.NodeRevoke(context.Background(), "n1", "cli:test", nil); err != nil {
				t.Fatal(err)
			}
		}, func(r *rig, code string, m nodeMat) enroll.Request {
			return r.request(code, "n1", newNodeMat(t).withNodeKeyOf(m))
		}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, nil)
			m := newNodeMat(t)
			c.setup(r, m)
			ap := approving("policy:x", DefaultTrust())
			off := r.offer(OfferOptions{Approver: ap})
			q := c.req(r, off.Code, m)
			_, err := r.svc.Redeem(context.Background(), q, "198.51.100.7:1")
			if pki.ReasonOf(err) != c.reason {
				t.Fatalf("want %s, got %v", c.reason, err)
			}
			csrFP, _ := pki.CSRKeyFingerprint(q.CSR)
			rows := r.audit("pairing.refuse")
			if len(rows) != 1 || rows[0].Target != off.ID {
				t.Fatalf("audit %+v", rows)
			}
			for _, want := range []string{"node=n1", "csr=" + csrFP, "reason=" + c.reason} {
				if !strings.Contains(rows[0].Detail, want) {
					t.Fatalf("audit detail %q lacks %q", rows[0].Detail, want)
				}
			}
			r.noCodeInAudit(off.Code)
			var takeover []Alert
			for _, a := range r.alerts.all() {
				if a.Kind == "pairing.takeover_attempt" {
					takeover = append(takeover, a)
				}
			}
			if len(takeover) != 1 || takeover[0].OfferID != off.ID || takeover[0].NodeID != "n1" {
				t.Fatalf("alerts %+v", r.alerts.all())
			}
			if len(ap.calls()) != 0 {
				t.Fatal("the approver was called")
			}
		})
	}
}

// TestOpusProbePendingPin: a pending pin (after `rotate --node-key`) is only finalized by an approved,
// bound redemption; a denial or a wrong key leaves it pending.
func enrollPendingPinSurvivesDenial(t *testing.T) {
	r := newRig(t, nil)
	old := newNodeMat(t)
	if _, err := r.redeem(r.offer(OfferOptions{Approver: approving("admin", DefaultTrust())}).Code, "n1", old); err != nil {
		t.Fatal(err)
	}
	next := newNodeMat(t)
	if err := r.ca.RotateNodeKey(context.Background(), "cli:test", "n1", pki.NodeKeyDigest(next.pub)); err != nil {
		t.Fatal(err)
	}
	pin := func() string { _, pub, _, _ := r.st.PKINodeKeyGet(context.Background(), "n1"); return pub }
	if pin() != "" {
		t.Fatal("rotate did not leave a pending pin")
	}
	_, err := r.redeem(r.offer(OfferOptions{Approver: denying("admin")}).Code, "n1", newNodeMat(t).withNodeKeyOf(next))
	e659(t, err, ReasonDenied, "")
	if pin() != "" {
		t.Fatal("a denied redemption consumed the pending pin")
	}
	if _, err = r.redeem(r.offer(OfferOptions{Approver: approving("admin", DefaultTrust())}).Code, "n1", newNodeMat(t).withNodeKeyOf(old)); pki.ReasonOf(err) != pki.ReasonNodeKeyChanged {
		t.Fatalf("the old key after rotate: %v", err)
	}
	if pin() != "" {
		t.Fatal("a refused redemption changed the pending pin")
	}
	if _, err = r.redeem(r.offer(OfferOptions{Approver: approving("admin", DefaultTrust())}).Code, "n1", newNodeMat(t).withNodeKeyOf(next)); err != nil {
		t.Fatalf("the rotated key: %v", err)
	}
	if pin() == "" {
		t.Fatal("an approved redemption did not finalize the pin")
	}
	if _, err = r.redeem(r.offer(OfferOptions{Approver: approving("admin", DefaultTrust())}).Code, "n1", newNodeMat(t).withNodeKeyOf(old)); pki.ReasonOf(err) != pki.ReasonNodeKeyChanged {
		t.Fatalf("the old key once the new one is pinned: %v", err)
	}
}

// TestOpusProbeForeignOfferBurned: an offer another process owns is refused without being consumed.
func enrollForeignOfferNotBurned(t *testing.T) {
	r := newRig(t, nil)
	other := New(r.svc.cfg)
	ap := approving("admin", DefaultTrust())
	off, err := other.Offer(context.Background(), OfferOptions{Approver: ap})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ { // more than the 5 wrong attempts that would lock it
		_, err = r.redeem(off.Code, "n1", newNodeMat(t))
		e659(t, err, ReasonNoApprover, off.Code)
	}
	r.untouched(off)
	if len(r.audit("pairing.refuse")) != 0 || len(r.alerts.all()) != 0 {
		t.Fatalf("audit %+v alerts %+v", r.audit(""), r.alerts.all())
	}
	if _, err = other.Redeem(context.Background(), r.request(off.Code, "n1", newNodeMat(t)), "127.0.0.1:1"); err != nil {
		t.Fatalf("the owning service could not redeem: %v", err)
	}
	if len(ap.calls()) != 1 || len(r.alerts.all()) != 0 {
		t.Fatalf("approver %d alerts %+v", len(ap.calls()), r.alerts.all())
	}
}

func enrollJoinDoesNotFollowRedirects(t *testing.T) {
	for _, phase := range []string{enroll.PhaseHello, enroll.PhaseEnroll} {
		r := newRig(t, nil)
		off := r.offer(OfferOptions{Approver: approving("policy:p", DefaultTrust())})
		var target atomic.Int32
		var targetBody atomic.Value
		leaf, err := r.ca.IssueServer("local", nil, loopback, 0)
		if err != nil {
			t.Fatal(err)
		}
		tgt, err := enroll.Listen("127.0.0.1:0", r.ca.EnrollTLSConfig(leaf), http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
			target.Add(1)
			b := make([]byte, 1<<16)
			n, _ := q.Body.Read(b)
			targetBody.Store(string(b[:n]))
		}))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tgt.Close)
		dest := "https://" + tgt.Addr().String() + enroll.Route
		front, err := enroll.Listen("127.0.0.1:0", r.ca.EnrollTLSConfig(leaf), http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
			var body enroll.Request
			_ = decodeJSON(q, &body)
			if body.Phase == phase {
				http.Redirect(w, q, dest, http.StatusTemporaryRedirect)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":1,"coordinator_id":"local","server_time":` + itoa(r.clock.Now().Unix()) + `}`))
		}))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(front.Close)
		r.clock.Advance(2e9)
		out, asked := &syncBuf{}, false
		opts, _ := r.joinOpts("https://"+front.Addr().String()+enroll.Route, off.Code, off.RootFingerprint, "node-r", out)
		opts.Code = func() (string, error) { asked = true; return off.Code, nil }
		_, err = runJoin(opts)
		if err == nil {
			t.Fatalf("%s: join followed a redirect and succeeded", phase)
		}
		if target.Load() != 0 {
			t.Fatalf("%s: the redirect target received %d requests (body %v)", phase, target.Load(), targetBody.Load())
		}
		if phase == enroll.PhaseHello && (asked || strings.Contains(out.String(), "CSR key")) {
			t.Fatal("a redirected hello still led to a CSR or the code")
		}
		if strings.Contains(err.Error(), off.Code) {
			t.Fatal("the error holds the code")
		}
	}
}

func decodeJSON(q *http.Request, v any) error {
	return jsonDecode(q.Body, v)
}
