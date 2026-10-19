package pairing

// Purpose: TestWithdraw: Withdraw revokes the certificate an offer produced (deny list, audit row, a fresh
// mTLS handshake fails) and ends an unredeemed offer.
// Inputs:  a rig and a node-kind mTLS server guarded by pki.Guard. Outputs: pass/fail.
// Constraints: TLS 1.3 lets the client finish first, so a refused client certificate shows on the first read.

import (
	"context"
	"crypto/tls"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// nodeServer is an mTLS listener that accepts node certificates of r's CA and checks the deny list.
func (r *rig) nodeServer() string {
	r.tb.Helper()
	leaf, err := r.ca.IssueServer("local", nil, loopback, 0)
	if err != nil {
		r.tb.Fatal(err)
	}
	cfg, err := r.ca.TLSServerConfig(pki.KindNode, leaf, pki.NewGuard(r.st))
	if err != nil {
		r.tb.Fatal(err)
	}
	return okServer(r.tb, cfg)
}

func nodeCert(resp enroll.Response, m nodeMat) *tls.Certificate {
	return &tls.Certificate{Certificate: [][]byte{resp.Cert, resp.Chain[1]}, PrivateKey: m.key}
}

func TestWithdraw(t *testing.T) {
	ctx := context.Background()
	t.Run("Withdraw after redemption revokes the issued certificate, writes an audit row and a fresh handshake fails", func(t *testing.T) {
		r := newRig(t, nil)
		ap := approving("policy:w", DefaultTrust())
		off := r.offer(OfferOptions{Approver: ap})
		m := newNodeMat(t)
		resp, err := r.redeem(off.Code, "node-w", m)
		if err != nil {
			t.Fatal(err)
		}
		addr := r.nodeServer()
		if err = dialOK(addr, r.clientCfg(nodeCert(resp, m))); err != nil {
			t.Fatalf("positive control: the fresh certificate was refused: %v", err)
		}
		if err = r.svc.Withdraw(ctx, off.Code); err != nil {
			t.Fatal(err)
		}
		serial := hexSerial(t, resp.Cert)
		if denied, _ := r.st.PKIIsDenied(ctx, serial); !denied {
			t.Fatal("the certificate is not on the deny list")
		}
		if err = dialOK(addr, r.clientCfg(nodeCert(resp, m))); err == nil {
			t.Fatal("a fresh handshake with the withdrawn certificate succeeded")
		}
		rows := r.audit("pairing.withdraw")
		if len(rows) != 1 || rows[0].Target != off.ID || !strings.Contains(rows[0].Detail, "revoked=true") || rows[0].Actor != "cli:test" {
			t.Fatalf("withdraw audit %+v", rows)
		}
		if d := r.audit("pki.deny"); len(d) != 1 || !strings.Contains(d[0].Detail, "reason=offer_withdrawn") || d[0].Target != "node-w" {
			t.Fatalf("deny audit %+v", d)
		}
		if _, err = r.ca.Renew(ctx, mustParse(t, resp.Cert), m.csr); pki.ReasonOf(err) != pki.ReasonRevoked {
			t.Fatalf("a withdrawn certificate renewed: %v", err)
		}
		_, err = r.redeem(off.Code, "node-w2", newNodeMat(t))
		e659(t, err, ReasonReused, off.Code) // a redeemed code stays redeemed
		r.noCodeInAudit(off.Code)
	})

	t.Run("a withdrawn unredeemed code cannot be redeemed and the approver is never asked", func(t *testing.T) {
		r := newRig(t, nil)
		ap := approving("policy:w", DefaultTrust())
		off := r.offer(OfferOptions{Approver: ap})
		other := r.offer(OfferOptions{Approver: ap})
		if err := r.svc.Withdraw(ctx, off.Code); err != nil {
			t.Fatal(err)
		}
		_, err := r.redeem(off.Code, "node-w", newNodeMat(t))
		e659(t, err, ReasonWithdrawn, off.Code)
		if len(ap.calls()) != 0 || r.count(`SELECT count(*) FROM pki_cert`) != 0 {
			t.Fatal("a withdrawn offer reached the approver or issued")
		}
		rows := r.audit("pairing.withdraw")
		if len(rows) != 1 || !strings.Contains(rows[0].Detail, "revoked=false") || len(r.audit("pki.deny")) != 0 {
			t.Fatalf("audit %+v", r.audit(""))
		}
		if _, err = r.redeem(other.Code, "node-ok", newNodeMat(t)); err != nil {
			t.Fatalf("withdrawing one offer broke another: %v", err)
		}
	})

	t.Run("withdrawing the offer of an open window ends the window and closes the listener", func(t *testing.T) {
		r := newRig(t, nil)
		w, wait := r.window(OfferOptions{Approver: approving("policy:w", DefaultTrust())})
		if !isOpen(w.Addr.String()) {
			t.Fatal("the window did not bind")
		}
		start := time.Now()
		if err := r.svc.Withdraw(ctx, w.Offer.Code); err != nil {
			t.Fatal(err)
		}
		if wr := r.waitFor(wait, time.Second); wr.redeemed || wr.err != nil {
			t.Fatalf("Wait = %+v", wr)
		}
		eventually(t, time.Second, "the listener closes", func() bool { return !isOpen(w.Addr.String()) })
		if time.Since(start) > time.Second {
			t.Fatal("closing took longer than 1 s")
		}
	})

	t.Run("Withdraw of an unknown code is an error and changes nothing", func(t *testing.T) {
		r := newRig(t, nil)
		off := r.offer(OfferOptions{Approver: approving("policy:w", DefaultTrust())})
		if err := r.svc.Withdraw(ctx, "AAAAAAAAAA"); err == nil {
			t.Fatal("withdrawing an unknown code succeeded")
		}
		if live, _ := r.st.PKIOffersLive(ctx, false); len(live) != 1 || live[0].ID != off.ID || len(r.audit("pairing.withdraw")) != 0 {
			t.Fatalf("live %+v audit %+v", live, r.audit(""))
		}
	})

	t.Run("a typed code with spaces and dashes withdraws the same offer", func(t *testing.T) {
		r := newRig(t, nil)
		off := r.offer(OfferOptions{Approver: approving("policy:w", DefaultTrust())})
		typed := strings.ToLower(off.Code[:5] + " - " + off.Code[5:])
		if err := r.svc.Withdraw(ctx, typed); err != nil {
			t.Fatal(err)
		}
		_, err := r.redeem(off.Code, "node-w", newNodeMat(t))
		e659(t, err, ReasonWithdrawn, off.Code)
	})
}
