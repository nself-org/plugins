package pairing

// Purpose: TestServeWhileOffers: the burst listener of `ci serve` binds only while an intent-bound offer
// is live and closes within 1 s of the last one ending; a code without a live intent is E659 no_intent
// and reaches no approver. Real loopback sockets on a fixed free port, probed with plain dials.
// Inputs:  a rig with the fake clock. Outputs: pass/fail. Constraints: closing is asserted within 1 s.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// serveBurst runs ServeWhileOffers on a fixed port; stop ends it and waits for its return.
func (r *rig) serveBurst() (addr, url string, stop func()) {
	r.tb.Helper()
	addr = freeAddr(r.tb)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.svc.ServeWhileOffers(ctx, addr, "https://127.0.0.1") }()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					r.tb.Errorf("ServeWhileOffers: %v", err)
				}
			case <-time.After(3 * time.Second):
				r.tb.Error("ServeWhileOffers did not return after cancel")
			}
		})
	}
	r.tb.Cleanup(stop)
	return addr, "https://" + addr + "/v1/enroll", stop
}

func settle() { time.Sleep(150 * time.Millisecond) } // several polls of 20 ms

func TestServeWhileOffers(t *testing.T) {
	ctx := context.Background()
	t.Run("binds no socket while no intent-bound offer is live", func(t *testing.T) {
		r := newRig(t, nil)
		addr, _, stop := r.serveBurst()
		settle()
		if isOpen(addr) {
			t.Fatal("a socket is bound with no offer")
		}
		r.offer(OfferOptions{Approver: approving("policy:s", DefaultTrust())}) // no Intent: never opens it
		settle()
		if isOpen(addr) {
			t.Fatal("an offer without an intent opened the listener")
		}
		stop()
	})

	t.Run("creating an intent-bound offer opens the listener and cancel closes it", func(t *testing.T) {
		r := newRig(t, nil)
		addr, _, stop := r.serveBurst()
		settle()
		r.offer(OfferOptions{Approver: approving("policy:s", DefaultTrust()), Intent: "burst-1"})
		eventually(t, time.Second, "the listener opens", func() bool { return isOpen(addr) })
		stop()
		if isOpen(addr) {
			t.Fatal("the listener outlived ServeWhileOffers")
		}
	})

	t.Run("redeeming the last offer closes the listener within 1 s", func(t *testing.T) {
		r := newRig(t, nil)
		ap := approving("policy:s", DefaultTrust())
		addr, url, _ := r.serveBurst()
		off := r.offer(OfferOptions{Approver: ap, Intent: "burst-1", Label: "ephemeral"})
		eventually(t, time.Second, "the listener opens", func() bool { return isOpen(addr) })
		res, err := r.join(url, off.Code, off.RootFingerprint, "node-b")
		if err != nil || res.NodeID != "node-b" {
			t.Fatalf("join: %+v %v", res, err)
		}
		eventually(t, time.Second, "the listener closes after the last redemption", func() bool { return !isOpen(addr) })
		if got := ap.calls(); len(got) != 1 || got[0].Intent != "burst-1" {
			t.Fatalf("approver saw %+v", got)
		}
	})

	t.Run("expiry of the last offer closes the listener within 1 s; an earlier expiry does not", func(t *testing.T) {
		r := newRig(t, func(c *Config) { c.Poll = 0 }) // the default 250 ms poll
		ap := approving("policy:s", DefaultTrust())
		addr, _, _ := r.serveBurst()
		r.offer(OfferOptions{Approver: ap, Intent: "a", TTL: 5 * time.Minute})
		r.offer(OfferOptions{Approver: ap, Intent: "b"})
		eventually(t, time.Second, "the listener opens", func() bool { return isOpen(addr) })
		r.clock.Advance(6 * time.Minute) // offer a ends, b is still live
		time.Sleep(600 * time.Millisecond)
		if !isOpen(addr) {
			t.Fatal("the listener closed while an intent-bound offer was still live")
		}
		r.clock.Advance(5 * time.Minute) // b ends too
		eventually(t, time.Second, "the listener closes after the last expiry", func() bool { return !isOpen(addr) })
		r.offer(OfferOptions{Approver: ap, Intent: "c"}) // a new offer re-opens it
		eventually(t, time.Second, "the listener re-opens", func() bool { return isOpen(addr) })
	})

	t.Run("withdrawing the last offer closes the listener within 1 s", func(t *testing.T) {
		r := newRig(t, nil)
		addr, _, _ := r.serveBurst()
		off := r.offer(OfferOptions{Approver: approving("policy:s", DefaultTrust()), Intent: "x"})
		eventually(t, time.Second, "the listener opens", func() bool { return isOpen(addr) })
		start := time.Now()
		if err := r.svc.Withdraw(ctx, off.Code); err != nil {
			t.Fatal(err)
		}
		eventually(t, time.Second, "the listener closes after the withdraw", func() bool { return !isOpen(addr) })
		if time.Since(start) > time.Second {
			t.Fatal("closing took longer than 1 s")
		}
	})

	t.Run("a code without a live intent is E659 no_intent: no approver call, no CSR processed", func(t *testing.T) {
		r := newRig(t, nil)
		addr, url, _ := r.serveBurst()
		holder := approving("policy:holder", DefaultTrust())
		r.offer(OfferOptions{Approver: holder, Intent: "keeps-it-open"})
		plain, withdrawn := approving("policy:plain", DefaultTrust()), approving("policy:withdrawn", DefaultTrust())
		noIntent := r.offer(OfferOptions{Approver: plain})
		gone := r.offer(OfferOptions{Approver: withdrawn, Intent: "to-withdraw"})
		if err := r.svc.Withdraw(ctx, gone.Code); err != nil {
			t.Fatal(err)
		}
		eventually(t, time.Second, "the listener opens", func() bool { return isOpen(addr) })
		for name, c := range map[string]OfferResult{"offer without an intent": noIntent, "offer bound to a withdrawn intent": gone} {
			_, err := r.join(url, c.Code, c.RootFingerprint, "node-n")
			wantCoded(t, err, "E659", ReasonNoIntent)
			if strings.Contains(err.Error(), c.Code) {
				t.Fatalf("%s: the error holds the code", name)
			}
		}
		if len(plain.calls())+len(withdrawn.calls())+len(holder.calls()) != 0 {
			t.Fatal("an approver was called for a refused code")
		}
		if r.count(`SELECT count(*) FROM pki_cert`) != 0 || r.count(`SELECT count(*) FROM pki_node_key`) != 0 || len(r.audit("pairing.approve"))+len(r.audit("pairing.deny")) != 0 {
			t.Fatal("a refused code issued, pinned or audited a decision")
		}
		if _, err := r.reg.Get(ctx, "node-n"); err == nil {
			t.Fatal("the node was registered")
		}
		if live, _ := r.st.PKIOffersLive(ctx, false); len(live) != 2 { // holder and the plain offer: no_intent consumed nothing
			t.Fatalf("live offers %+v", live)
		}
	})

	t.Run("the approver passed with that offer decides, never another offer's", func(t *testing.T) {
		r := newRig(t, nil)
		addr, url, _ := r.serveBurst()
		no, yes := denying("policy:no"), approving("policy:yes", DefaultTrust())
		offNo := r.offer(OfferOptions{Approver: no, Intent: "i-no"})
		offYes := r.offer(OfferOptions{Approver: yes, Intent: "i-yes"})
		eventually(t, time.Second, "the listener opens", func() bool { return isOpen(addr) })
		_, err := r.join(url, offNo.Code, offNo.RootFingerprint, "node-no")
		wantCoded(t, err, "E659", ReasonDenied)
		if res, err := r.join(url, offYes.Code, offYes.RootFingerprint, "node-yes"); err != nil || res.NodeID != "node-yes" {
			t.Fatalf("approved join: %+v %v", res, err)
		}
		if len(no.calls()) != 1 || len(yes.calls()) != 1 || no.calls()[0].Intent != "i-no" || yes.calls()[0].Intent != "i-yes" {
			t.Fatalf("no %+v yes %+v", no.calls(), yes.calls())
		}
		eventually(t, time.Second, "the listener closes when both offers are spent", func() bool { return !isOpen(addr) })
	})

	t.Run("an approval still in flight keeps the listener open until it answers", func(t *testing.T) {
		r := newRig(t, nil)
		addr, url, _ := r.serveBurst()
		ap := &blockingApprover{entered: make(chan struct{}), release: make(chan struct{}), dec: Decision{Approve: true, Trust: DefaultTrust()}}
		off := r.offer(OfferOptions{Approver: ap, Intent: "slow"})
		eventually(t, time.Second, "the listener opens", func() bool { return isOpen(addr) })
		done := make(chan error, 1)
		go func() { _, err := r.join(url, off.Code, off.RootFingerprint, "node-slow"); done <- err }()
		select {
		case <-ap.entered:
		case <-time.After(60 * time.Second):
			t.Fatal("the redemption never reached the approver")
		}
		settle()
		if !isOpen(addr) {
			t.Fatal("the listener closed under an in-flight approval")
		}
		close(ap.release)
		if err := <-done; err != nil {
			t.Fatalf("join: %v", err)
		}
		eventually(t, time.Second, "the listener closes after the answer", func() bool { return !isOpen(addr) })
	})

	t.Run("a listener that cannot bind is an error, not a spin", func(t *testing.T) {
		r := newRig(t, nil)
		addr := freeAddr(t)
		blocker := r.tbListen(addr)
		defer blocker.Close()
		r.offer(OfferOptions{Approver: approving("policy:s", DefaultTrust()), Intent: "x"})
		done := make(chan error, 1)
		go func() { done <- r.svc.ServeWhileOffers(ctx, addr, "https://127.0.0.1") }()
		select {
		case err := <-done:
			if err == nil || errors.Is(err, context.Canceled) {
				t.Fatalf("err %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("ServeWhileOffers kept running although it could not bind")
		}
	})
}

func TestServeWhileOffersIgnoresOffersOfAnotherProcess(t *testing.T) {
	r := newRig(t, nil)
	addr, _, _ := r.serveBurst()
	// A second Service on the same store stands for another process: it holds the approver, this one does not.
	other := New(Config{CA: r.ca, Store: r.st, Registry: r.reg, CoordinatorID: "local", Actor: "cli:other", Poll: 20 * time.Millisecond})
	ap := approving("policy:other", DefaultTrust())
	off, err := other.Offer(context.Background(), OfferOptions{Approver: ap, Intent: "theirs"})
	if err != nil {
		t.Fatal(err)
	}
	settle()
	settle()
	if isOpen(addr) {
		t.Fatal("the listener opened for an offer whose approver lives in another process")
	}
	live, _ := r.st.PKIOffersLive(context.Background(), true)
	if len(live) != 1 || live[0].Attempts != 0 || live[0].RedeemedAt != 0 {
		t.Fatalf("the offer was touched: %+v", live)
	}
	// Its own owner still redeems it: the code was not burned.
	if _, err = other.Redeem(context.Background(), r.request(off.Code, "node-other", newNodeMat(t)), "127.0.0.1:1"); err != nil {
		t.Fatalf("the owning process could not redeem: %v", err)
	}
	if len(ap.calls()) != 1 {
		t.Fatal("the owner's approver was not used")
	}
	// An offer made here opens the listener, and the foreign one does not keep it open afterwards.
	mine := r.offer(OfferOptions{Approver: approving("policy:mine", DefaultTrust()), Intent: "mine"})
	eventually(t, time.Second, "the listener opens for our own offer", func() bool { return isOpen(addr) })
	if err = r.svc.Withdraw(context.Background(), mine.Code); err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, "the listener closes", func() bool { return !isOpen(addr) })
}
