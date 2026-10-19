package pairing

// Purpose: TestApprover: the per-offer Approver (approve, deny, error, default, none, concurrent) driven
// through Redeem against a real store, CA and registry.
// Inputs:  a rig and fake approvers. Outputs: pass/fail. Constraints: -race clean; the audit rows are the
//          evidence: one per decision, never holding a code.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestApprover(t *testing.T) {
	t.Run("an approver that approves issues exactly the trust fields it returned and writes one audit row", func(t *testing.T) {
		for _, tf := range []TrustFields{
			{Accepts: []string{"owner"}, SecretClasses: []string{"none"}, PrivacyZone: "local-only", Projects: []string{"proj-1"}},
			{Accepts: []string{"internal", "collaborator"}, SecretClasses: []string{"none"}, PrivacyZone: "hosted-allowed"},
		} {
			r := newRig(t, nil)
			ap := approving("policy:lab", tf)
			off := r.offer(OfferOptions{Approver: ap})
			m := newNodeMat(t)
			resp, err := r.redeem(off.Code, "node-a", m)
			if err != nil {
				t.Fatal(err)
			}
			n, err := r.reg.Get(context.Background(), "node-a")
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprint(*n.Capability.Trust.Accepts.Value); got != fmt.Sprint(tf.Accepts) {
				t.Fatalf("accepts %s want %v", got, tf.Accepts)
			}
			if got := fmt.Sprint(*n.Capability.Trust.SecretClasses.Value); got != fmt.Sprint(tf.SecretClasses) {
				t.Fatalf("secret classes %s", got)
			}
			if got := string(*n.Capability.Trust.PrivacyZone.Value); got != tf.PrivacyZone {
				t.Fatalf("zone %s", got)
			}
			if got := len(n.Capability.Lifecycle.AuthorizedProjects); got != len(tf.Projects) {
				t.Fatalf("projects %v", n.Capability.Lifecycle.AuthorizedProjects)
			}
			rows := r.audit("pairing.approve")
			if len(rows) != 1 || len(r.audit("pairing.deny")) != 0 {
				t.Fatalf("audit %+v", r.audit(""))
			}
			d := rows[0].Detail
			for _, want := range []string{"approver=policy:lab", "csr=" + m.csrFP, "serial=" + hexSerial(t, resp.Cert),
				"accepts=" + strings.Join(tf.Accepts, ","), "secrets=none", "zone=" + tf.PrivacyZone, "projects=" + strings.Join(tf.Projects, ",")} {
				if !strings.Contains(d, want) {
					t.Fatalf("audit detail %q lacks %q", d, want)
				}
			}
			if rows[0].Target != off.ID || rows[0].Actor != "cli:test" {
				t.Fatalf("audit row %+v", rows[0])
			}
			got := ap.calls()
			if len(got) != 1 || got[0].CSRFingerprint != m.csrFP || got[0].OfferID != off.ID || got[0].CertTTL != 7*24*3600e9 {
				t.Fatalf("approval request %+v", got)
			}
			if strings.Contains(fmt.Sprintf("%+v", got[0]), off.Code) {
				t.Fatal("the approver was given the code")
			}
			r.noCodeInAudit(off.Code)
		}
	})

	t.Run("an approver that denies returns E659 denied, issues nothing and writes one audit row", func(t *testing.T) {
		r := newRig(t, nil)
		ap := denying("policy:strict")
		off := r.offer(OfferOptions{Approver: ap})
		m := newNodeMat(t)
		_, err := r.redeem(off.Code, "node-x", m)
		e659(t, err, ReasonDenied, off.Code)
		rows := r.audit("pairing.deny")
		if len(rows) != 1 || len(r.audit("pairing.approve")) != 0 {
			t.Fatalf("audit %+v", r.audit(""))
		}
		for _, want := range []string{"approver=policy:strict", "csr=" + m.csrFP, "node=node-x", "reason=no by policy:strict"} {
			if !strings.Contains(rows[0].Detail, want) {
				t.Fatalf("deny detail %q lacks %q", rows[0].Detail, want)
			}
		}
		if r.count(`SELECT count(*) FROM pki_cert`) != 0 {
			t.Fatal("a certificate was issued")
		}
		if _, gerr := r.reg.Get(context.Background(), "node-x"); gerr == nil {
			t.Fatal("the node was registered")
		}
		if r.count(`SELECT count(*) FROM pki_node_key`) != 0 {
			t.Fatal("a node key was pinned")
		}
		r.noCodeInAudit(off.Code)
	})

	t.Run("an approver that fails is a denial: nothing issued, one audit row", func(t *testing.T) {
		r := newRig(t, nil)
		ap := approving("policy:broken", DefaultTrust())
		ap.err = context.DeadlineExceeded
		off := r.offer(OfferOptions{Approver: ap})
		_, err := r.redeem(off.Code, "node-e", newNodeMat(t))
		e659(t, err, ReasonDenied, off.Code)
		rows := r.audit("pairing.deny")
		if len(rows) != 1 || !strings.Contains(rows[0].Detail, "reason=approver error") || r.count(`SELECT count(*) FROM pki_cert`) != 0 {
			t.Fatalf("audit %+v", r.audit(""))
		}
	})

	t.Run("the reason in the audit row is cut to 120 characters without control characters", func(t *testing.T) {
		r := newRig(t, nil)
		ap := denying("policy:long")
		ap.dec.Reason = strings.Repeat("x", 90) + "\n\t" + strings.Repeat("y", 200)
		off := r.offer(OfferOptions{Approver: ap})
		_, _ = r.redeem(off.Code, "node-l", newNodeMat(t))
		rows := r.audit("pairing.deny")
		d := rows[0].Detail
		reason := d[strings.Index(d, "reason=")+len("reason="):]
		if len(rows) != 1 || len(reason) != 120 || strings.ContainsAny(reason, "\n\t") {
			t.Fatalf("reason %q (%d)", reason, len(reason))
		}
	})

	t.Run("an offer without an Approver uses Config.Approver; one with its own approver never touches the default", func(t *testing.T) {
		def := approving("admin", DefaultTrust())
		r := newRig(t, func(c *Config) { c.Approver = def })
		own := approving("policy:own", DefaultTrust())
		a := r.offer(OfferOptions{})
		b := r.offer(OfferOptions{Approver: own})
		if _, err := r.redeem(a.Code, "node-def", newNodeMat(t)); err != nil {
			t.Fatal(err)
		}
		if len(def.calls()) != 1 || len(own.calls()) != 0 {
			t.Fatalf("default %d own %d", len(def.calls()), len(own.calls()))
		}
		if _, err := r.redeem(b.Code, "node-own", newNodeMat(t)); err != nil {
			t.Fatal(err)
		}
		if len(def.calls()) != 1 || len(own.calls()) != 1 {
			t.Fatalf("default %d own %d", len(def.calls()), len(own.calls()))
		}
		if d := r.audit("pairing.approve"); len(d) != 2 || !strings.Contains(d[0].Detail, "approver=admin") || !strings.Contains(d[1].Detail, "approver=policy:own") {
			t.Fatalf("audit %+v", d)
		}
	})

	t.Run("an Offer with no approver at all fails no_approver and creates no offer", func(t *testing.T) {
		r := newRig(t, nil)
		_, err := r.svc.Offer(context.Background(), OfferOptions{})
		e659(t, err, ReasonNoApprover, "")
		if r.count(`SELECT count(*) FROM pki_offer`) != 0 {
			t.Fatal("an offer row was created")
		}
	})

	t.Run("an offer whose approver is gone (another process) is refused no_approver and issues nothing", func(t *testing.T) {
		r := newRig(t, nil)
		off := r.offer(OfferOptions{Approver: approving("policy:gone", DefaultTrust())})
		fresh := New(Config{CA: r.ca, Store: r.st, Registry: r.reg, CoordinatorID: "local", Actor: "cli:test"})
		_, err := fresh.Redeem(context.Background(), r.request(off.Code, "node-g", newNodeMat(t)), "127.0.0.1:1")
		e659(t, err, ReasonNoApprover, off.Code)
		if r.count(`SELECT count(*) FROM pki_cert`) != 0 {
			t.Fatal("a certificate was issued")
		}
	})

	t.Run("two concurrent offers with different approvers never use each other's callback", func(t *testing.T) {
		const n = 8
		r := newRig(t, nil)
		aps := make([]*fakeApprover, n)
		offers := make([]OfferResult, n)
		for i := range aps {
			aps[i] = approving(fmt.Sprintf("policy:%d", i), TrustFields{Accepts: []string{"owner"}, SecretClasses: []string{"none"}, Projects: []string{fmt.Sprintf("proj-%d", i)}})
			offers[i] = r.offer(OfferOptions{Approver: aps[i], Label: fmt.Sprintf("label-%d", i)})
		}
		reqs := make([]nodeMat, n)
		for i := range reqs {
			reqs[i] = newNodeMat(t)
		}
		var wg sync.WaitGroup
		errs := make([]error, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = r.svc.Redeem(context.Background(), r.request(offers[i].Code, fmt.Sprintf("node-%d", i), reqs[i]), fmt.Sprintf("127.0.0.%d:1", i+1))
			}()
		}
		wg.Wait()
		for i := range aps {
			if errs[i] != nil {
				t.Fatalf("redeem %d: %v", i, errs[i])
			}
			got := aps[i].calls()
			if len(got) != 1 || got[0].OfferID != offers[i].ID || got[0].Label != fmt.Sprintf("label-%d", i) || got[0].CSRFingerprint != reqs[i].csrFP {
				t.Fatalf("approver %d saw %+v", i, got)
			}
			node, err := r.reg.Get(context.Background(), fmt.Sprintf("node-%d", i))
			if err != nil {
				t.Fatal(err)
			}
			if p := node.Capability.Lifecycle.AuthorizedProjects; len(p) != 1 || p[0] != fmt.Sprintf("proj-%d", i) {
				t.Fatalf("node %d authorized %v", i, p)
			}
		}
		if len(r.audit("pairing.approve")) != n {
			t.Fatalf("%d approve rows", len(r.audit("pairing.approve")))
		}
	})
}
