package pairing

// Purpose: TestCertTTL: the certificate lifetime of an offer (30 m, default 7 days, the 1 minute..7 days
// bounds) and that a short lineage never renews.
// Inputs:  a rig. Outputs: pass/fail. Constraints: the clock is fixed during issuance, so NotAfter is exact.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

func TestCertTTL(t *testing.T) {
	ctx := context.Background()
	t.Run("an offer with CertTTL 30m issues NotAfter 30m after issuance and renewal never extends it", func(t *testing.T) {
		r := newRig(t, nil)
		ap := approving("policy:ttl", DefaultTrust())
		off := r.offer(OfferOptions{Approver: ap, CertTTL: 30 * time.Minute})
		if off.CertTTL != 30*time.Minute {
			t.Fatalf("offer result TTL %s", off.CertTTL)
		}
		m := newNodeMat(t)
		resp, err := r.redeem(off.Code, "node-short", m)
		if err != nil {
			t.Fatal(err)
		}
		leaf := mustParse(t, resp.Cert)
		if want := r.clock.Now().Add(30 * time.Minute); !leaf.NotAfter.Equal(want) {
			t.Fatalf("NotAfter %s want %s", leaf.NotAfter, want)
		}
		if resp.NotAfter != leaf.NotAfter.Unix() {
			t.Fatalf("response NotAfter %d", resp.NotAfter)
		}
		if got := ap.calls(); len(got) != 1 || got[0].CertTTL != 30*time.Minute {
			t.Fatalf("approver saw %+v", got)
		}
		for _, step := range []time.Duration{0, 19 * time.Minute, 10 * time.Minute} { // at issue, past the 2/3 mark (+19m), near the end (+29m)
			r.clock.Advance(step)
			_, err = r.ca.Renew(ctx, leaf, m.csr)
			if pki.ReasonOf(err) != pki.ReasonRenewCap {
				t.Fatalf("renewal %s later: %v", step, err)
			}
		}
		if n := r.count(`SELECT count(*) FROM pki_cert`); n != 1 {
			t.Fatalf("%d certificates after refused renewals", n)
		}
		r.clock.Advance(2 * time.Minute) // past NotAfter: expired, not renewable either
		if _, err = r.ca.Renew(ctx, leaf, m.csr); pki.ReasonOf(err) != pki.ReasonExpired {
			t.Fatalf("renewal after expiry: %v", err)
		}
	})

	t.Run("CertTTL 0 gives 7 days and that lineage renews", func(t *testing.T) {
		r := newRig(t, nil)
		off := r.offer(OfferOptions{Approver: approving("policy:ttl", DefaultTrust())})
		if off.CertTTL != 7*24*time.Hour {
			t.Fatalf("offer result TTL %s", off.CertTTL)
		}
		m := newNodeMat(t)
		resp, err := r.redeem(off.Code, "node-week", m)
		if err != nil {
			t.Fatal(err)
		}
		leaf := mustParse(t, resp.Cert)
		if want := r.clock.Now().Add(7 * 24 * time.Hour); !leaf.NotAfter.Equal(want) {
			t.Fatalf("NotAfter %s want %s", leaf.NotAfter, want)
		}
		r.clock.Advance(5 * 24 * time.Hour)
		next, err := r.ca.Renew(ctx, leaf, newNodeMat(t).csr)
		if err != nil {
			t.Fatalf("renew: %v", err)
		}
		if !next.NotAfter.Equal(r.clock.Now().Add(7 * 24 * time.Hour)) {
			t.Fatalf("renewed NotAfter %s", next.NotAfter)
		}
	})

	t.Run("CertTTL 8 days and 30 s are rejected by Offer with E660 cert_ttl and no offer is created", func(t *testing.T) {
		r := newRig(t, nil)
		for _, ttl := range []time.Duration{8 * 24 * time.Hour, 7*24*time.Hour + time.Nanosecond, 30 * time.Second, time.Minute - time.Nanosecond, -time.Minute} {
			_, err := r.svc.Offer(ctx, OfferOptions{Approver: approving("policy:ttl", DefaultTrust()), CertTTL: ttl})
			wantCoded(t, err, "E660", pki.ReasonCertTTL)
		}
		if n := r.count(`SELECT count(*) FROM pki_offer`); n != 0 {
			t.Fatalf("%d offer rows after rejected offers", n)
		}
		if live, _ := r.st.PKIOffersLive(ctx, false); len(live) != 0 {
			t.Fatalf("live offers %+v", live)
		}
	})

	t.Run("the bounds themselves, 1 minute and 7 days, are accepted", func(t *testing.T) {
		r := newRig(t, nil)
		for _, ttl := range []time.Duration{time.Minute, 7 * 24 * time.Hour} {
			off := r.offer(OfferOptions{Approver: approving("policy:ttl", DefaultTrust()), CertTTL: ttl})
			resp, err := r.redeem(off.Code, "node-"+ttl.String(), newNodeMat(t))
			if err != nil {
				t.Fatalf("%s: %v", ttl, err)
			}
			if got := mustParse(t, resp.Cert).NotAfter; !got.Equal(r.clock.Now().Add(ttl)) {
				t.Fatalf("%s: NotAfter %s", ttl, got)
			}
		}
	})

	t.Run("the offer TTL: 0 is 10 minutes, longer is refused", func(t *testing.T) {
		r := newRig(t, nil)
		ap := approving("policy:ttl", DefaultTrust())
		off := r.offer(OfferOptions{Approver: ap})
		if !off.ExpiresAt.Equal(r.clock.Now().Add(OfferTTL)) {
			t.Fatalf("expires %s", off.ExpiresAt)
		}
		for _, life := range []time.Duration{OfferTTL + time.Second, -time.Second} {
			_, err := r.svc.Offer(ctx, OfferOptions{Approver: ap, TTL: life})
			e659(t, err, ReasonBadRequest, "")
		}
		short := r.offer(OfferOptions{Approver: ap, TTL: time.Minute})
		r.clock.Advance(time.Minute + time.Second)
		_, err := r.redeem(short.Code, "node-late", newNodeMat(t))
		e659(t, err, ReasonExpired, short.Code)
	})

	t.Run("label and intent with control characters or excess length are refused", func(t *testing.T) {
		r := newRig(t, nil)
		ap := approving("policy:ttl", DefaultTrust())
		for _, o := range []OfferOptions{
			{Approver: ap, Label: "line\nbreak"}, {Approver: ap, Label: strings.Repeat("l", 65)},
			{Approver: ap, Intent: "tab\there"}, {Approver: ap, Intent: strings.Repeat("i", 129)},
		} {
			_, err := r.svc.Offer(ctx, o)
			e659(t, err, ReasonBadRequest, "")
		}
		if r.count(`SELECT count(*) FROM pki_offer`) != 0 {
			t.Fatal("an offer row was created")
		}
	})
}
