package pairing

// Purpose: the code-state cases of TestEnroll: distinct E659 reason keys, lockout after 5 wrong codes,
// reuse (revoke, audit, alert), and the requests that must not consume a code (skew, malformed).
// Inputs:  a rig. Outputs: pass/fail. Constraints: direct Redeem calls for the state machine, the real
//          listener for the skew path.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

func e659(t *testing.T, err error, reason string, code string) {
	t.Helper()
	var pe *Error
	if !errors.As(err, &pe) || pe.ErrCode() != "E659" || pe.Reason != reason {
		t.Fatalf("want E659 %s, got %v", reason, err)
	}
	if code != "" && strings.Contains(err.Error(), code) {
		t.Fatalf("the error message holds the code: %v", err)
	}
}

func enrollReasons(t *testing.T) {
	seen := map[string]bool{}
	note := func(err error, want, code string) {
		t.Helper()
		e659(t, err, want, code)
		if seen[want] {
			t.Fatalf("reason %q used twice", want)
		}
		seen[want] = true
	}
	ap := approving("policy:r", DefaultTrust())

	r := newRig(t, nil)
	_, err := r.redeem("AAAAAAAAAA", "n-invalid", newNodeMat(t))
	note(err, ReasonInvalid, "AAAAAAAAAA")

	r = newRig(t, nil)
	off := r.offer(OfferOptions{Approver: ap})
	r.clock.Advance(OfferTTL + time.Second)
	_, err = r.redeem(off.Code, "n-expired", newNodeMat(t))
	note(err, ReasonExpired, off.Code)

	r = newRig(t, nil)
	off = r.offer(OfferOptions{Approver: ap})
	if err = r.svc.Withdraw(context.Background(), off.Code); err != nil {
		t.Fatal(err)
	}
	_, err = r.redeem(off.Code, "n-withdrawn", newNodeMat(t))
	note(err, ReasonWithdrawn, off.Code)

	r = newRig(t, nil)
	off = r.offer(OfferOptions{Approver: ap})
	for i := 0; i < 5; i++ {
		_, err = r.redeem("BBBBBBBBB"+string(rune('A'+i)), "n-locked", newNodeMat(t))
		e659(t, err, ReasonInvalid, "")
	}
	_, err = r.redeem(off.Code, "n-locked", newNodeMat(t))
	note(err, ReasonLocked, off.Code)

	r = newRig(t, nil)
	off = r.offer(OfferOptions{Approver: ap})
	if _, err = r.redeem(off.Code, "n-first", newNodeMat(t)); err != nil {
		t.Fatal(err)
	}
	_, err = r.redeem(off.Code, "n-second", newNodeMat(t))
	note(err, ReasonReused, off.Code)

	r = newRig(t, nil)
	off = r.offer(OfferOptions{Approver: denying("policy:no")})
	_, err = r.redeem(off.Code, "n-denied", newNodeMat(t))
	note(err, ReasonDenied, off.Code)

	r = newRig(t, nil)
	off = r.offer(OfferOptions{Approver: ap})
	_, err = r.svc.RedeemIntent(context.Background(), r.request(off.Code, "n-intent", newNodeMat(t)), "127.0.0.1:1")
	note(err, ReasonNoIntent, off.Code)

	if len(seen) != 7 {
		t.Fatalf("reasons %v", seen)
	}
}

func enrollLockBoundary(t *testing.T) {
	r := newRig(t, nil)
	ap := approving("policy:r", DefaultTrust())
	off := r.offer(OfferOptions{Approver: ap})
	other := r.offer(OfferOptions{Approver: ap})
	for i := 0; i < 4; i++ {
		_, err := r.redeem("CCCCCCCCC"+string(rune('A'+i)), "n-b", newNodeMat(t))
		e659(t, err, ReasonInvalid, "")
	}
	// A wrong code is a wrong attempt on EVERY live offer: the fifth locks both.
	_, err := r.redeem("CCCCCCCCCE", "n-b", newNodeMat(t))
	e659(t, err, ReasonInvalid, "")
	for _, o := range []OfferResult{off, other} {
		_, err = r.redeem(o.Code, "n-b", newNodeMat(t))
		e659(t, err, ReasonLocked, o.Code)
	}
	if len(ap.calls()) != 0 {
		t.Fatal("a locked offer reached the approver")
	}

	r = newRig(t, nil)
	off = r.offer(OfferOptions{Approver: ap})
	for i := 0; i < 4; i++ {
		_, _ = r.redeem("DDDDDDDDD"+string(rune('A'+i)), "n-b", newNodeMat(t))
	}
	if _, err = r.redeem(off.Code, "n-b", newNodeMat(t)); err != nil {
		t.Fatalf("4 wrong attempts must not lock: %v", err)
	}
}

func enrollReuseRevokes(t *testing.T) {
	r := newRig(t, nil)
	ap := approving("policy:r", DefaultTrust())
	off := r.offer(OfferOptions{Approver: ap})
	first, err := r.redeem(off.Code, "node-one", newNodeMat(t))
	if err != nil {
		t.Fatal(err)
	}
	serial := hexSerial(t, first.Cert)
	ctx := context.Background()
	if denied, _ := r.st.PKIIsDenied(ctx, serial); denied {
		t.Fatal("denied before the reuse")
	}
	_, err = r.redeem(off.Code, "node-two", newNodeMat(t))
	e659(t, err, ReasonReused, off.Code)
	if denied, _ := r.st.PKIIsDenied(ctx, serial); !denied {
		t.Fatal("the first certificate is not on the deny list")
	}
	rows := r.audit("pairing.reuse")
	if len(rows) != 1 || rows[0].Target != off.ID || !strings.Contains(rows[0].Detail, "revoked=true") {
		t.Fatalf("reuse audit %+v", rows)
	}
	if len(r.audit("pki.deny")) != 1 {
		t.Fatalf("deny audit %+v", r.audit("pki.deny"))
	}
	al := r.alerts.all()
	if len(al) != 1 || al[0] != (Alert{Kind: "pairing.code_reuse", OfferID: off.ID, NodeID: "node-one", Serial: serial}) {
		t.Fatalf("alerts %+v", al)
	}
	if certs, _ := r.st.PKICertsOfNode(ctx, "node-two"); len(certs) != 0 {
		t.Fatal("a certificate was issued to the second presenter")
	}
	if _, gerr := r.reg.Get(ctx, "node-two"); gerr == nil {
		t.Fatal("the second presenter was registered")
	}
	if len(ap.calls()) != 1 {
		t.Fatalf("approver called %d times", len(ap.calls()))
	}
	r.noCodeInAudit(off.Code)
}

func enrollSkewServer(t *testing.T) {
	r := newRig(t, nil)
	ap := approving("policy:s", DefaultTrust())
	off := r.offer(OfferOptions{Approver: ap})
	for _, skew := range []time.Duration{pki.ClockSkew + time.Second, -pki.ClockSkew - time.Second} {
		req := r.request(off.Code, "node-s", newNodeMat(t))
		req.ClientTime = r.clock.Now().Add(skew).Unix()
		_, err := r.svc.Redeem(context.Background(), req, "127.0.0.1:1")
		if pki.ReasonOf(err) != pki.ReasonClockSkew || !strings.Contains(err.Error(), "E660") {
			t.Fatalf("skew %s: %v", skew, err)
		}
	}
	if len(ap.calls()) != 0 || r.count(`SELECT count(*) FROM pki_offer WHERE redeemed_at>0 OR attempts>0`) != 0 {
		t.Fatal("a skewed request touched the offer")
	}
	// Exactly 120 s is inside the window, and the code still works.
	req := r.request(off.Code, "node-s", newNodeMat(t))
	req.ClientTime = r.clock.Now().Add(pki.ClockSkew).Unix()
	if _, err := r.svc.Redeem(context.Background(), req, "127.0.0.1:1"); err != nil {
		t.Fatalf("skew of exactly 120 s: %v", err)
	}
}

func enrollSkewClient(t *testing.T) {
	r := newRig(t, nil)
	ap := approving("policy:s", DefaultTrust())
	off := r.offer(OfferOptions{Approver: ap})
	real := r.serveReal()
	// Fast: the hello answers and join compares times. Slow: the server leaf (backdated 1 minute) is not
	// valid yet at the node's time, which must also surface as clock_skew, not as a bare x509 error.
	for _, shift := range []time.Duration{200 * time.Second, -200 * time.Second} {
		r.clock.Advance(2 * time.Second)
		out := &syncBuf{}
		opts, _ := r.joinOpts(real.URL, off.Code, off.RootFingerprint, "node-c", out)
		opts.Now = func() time.Time { return r.clock.Now().Add(shift) }
		asked := false
		opts.Code = func() (string, error) { asked = true; return off.Code, nil }
		_, err := runJoin(opts)
		wantCoded(t, err, "E660", pki.ReasonClockSkew)
		if asked || strings.Contains(out.String(), "CSR key fingerprint") || real.Cnt.redeem.Load() != 0 || len(ap.calls()) != 0 {
			t.Fatalf("shift %s: a skewed node sent a CSR or asked for the code: asked %v out %q", shift, asked, out.String())
		}
	}
	if real.Cnt.hellos.Load() != 1 {
		t.Fatalf("hellos = %d, want 1 (only the fast clock reaches the hello)", real.Cnt.hellos.Load())
	}
	// Just inside the window is fine, and the code is still good after the refusals.
	r.clock.Advance(2 * time.Second)
	opts, _ := r.joinOpts(real.URL, off.Code, off.RootFingerprint, "node-c", &syncBuf{})
	opts.Now = func() time.Time { return r.clock.Now().Add(100 * time.Second) }
	if _, err := runJoin(opts); err != nil {
		t.Fatalf("a clock 100 s off must join, and the code must have survived: %v", err)
	}
}

func enrollMalformedKeepsCode(t *testing.T) {
	r := newRig(t, nil)
	ap := approving("policy:m", DefaultTrust())
	off := r.offer(OfferOptions{Approver: ap})
	good := newNodeMat(t)
	bad := []struct {
		name   string
		mut    func(*enroll.Request)
		reason string
		e660   bool
	}{
		{"short node key", func(q *enroll.Request) { q.NodeKey = q.NodeKey[:5] }, ReasonBadRequest, false},
		{"bad node name", func(q *enroll.Request) { q.Name = "Bad Name!" }, ReasonBadRequest, false},
		{"empty node name", func(q *enroll.Request) { q.Name = "" }, ReasonBadRequest, false},
		{"malformed code shape", func(q *enroll.Request) { q.Code = "short" }, ReasonBadRequest, false},
		{"garbage CSR", func(q *enroll.Request) { q.CSR = []byte("not a csr") }, pki.ReasonBadCSR, true},
		{"empty CSR", func(q *enroll.Request) { q.CSR = nil }, pki.ReasonBadCSR, true},
	}
	for _, tc := range bad {
		req := r.request(off.Code, "node-m", good)
		tc.mut(&req)
		_, err := r.svc.Redeem(context.Background(), req, "127.0.0.1:1")
		if tc.e660 {
			if pki.ReasonOf(err) != tc.reason {
				t.Fatalf("%s: %v", tc.name, err)
			}
		} else {
			e659(t, err, tc.reason, off.Code)
		}
	}
	if len(ap.calls()) != 0 || r.count(`SELECT count(*) FROM pki_offer WHERE redeemed_at>0 OR attempts>0`) != 0 {
		t.Fatal("a malformed request touched the offer")
	}
	if _, err := r.redeem(off.Code, "node-m", good); err != nil {
		t.Fatalf("the code did not survive the malformed requests: %v", err)
	}
}

func enrollReuseDuringApproval(t *testing.T) {
	r := newRig(t, nil)
	ap := &blockingApprover{entered: make(chan struct{}), release: make(chan struct{}), dec: Decision{Approve: true, Trust: DefaultTrust()}}
	off := r.offer(OfferOptions{Approver: ap})
	type result struct {
		resp enroll.Response
		err  error
	}
	first := make(chan result, 1)
	go func() {
		resp, err := r.redeem(off.Code, "node-race", newNodeMat(t))
		first <- result{resp, err}
	}()
	select {
	case <-ap.entered:
	case <-time.After(60 * time.Second):
		t.Fatal("the first redemption never reached the approver")
	}
	_, err := r.redeem(off.Code, "node-thief", newNodeMat(t))
	e659(t, err, ReasonReused, off.Code)
	close(ap.release) // the approval of the first presenter now completes
	var got result
	select {
	case got = <-first:
	case <-time.After(60 * time.Second):
		t.Fatal("the first redemption never finished")
	}
	e659(t, got.err, ReasonReused, off.Code)
	certs, _ := r.st.PKICertsOfNode(context.Background(), "node-race")
	if len(certs) != 1 {
		t.Fatalf("certs %+v", certs)
	}
	if denied, _ := r.st.PKIIsDenied(context.Background(), certs[0].Serial); !denied {
		t.Fatal("a certificate issued for a tainted code was not revoked at once")
	}
	if len(r.audit("pairing.approve")) != 0 {
		t.Fatalf("an approve row was written for a revoked issuance: %+v", r.audit("pairing.approve"))
	}
	if len(r.alerts.all()) != 1 {
		t.Fatalf("alerts %+v", r.alerts.all())
	}
}
