package pairing

// Purpose: the small surfaces of the package: the join-hook registry, the package-level entry points
// (Use, Offer, Withdraw, ServeWhileOffers) and the capability draft a paired node starts from.
// Inputs:  none. Outputs: pass/fail. Constraints: the default service is restored to none after each test.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
)

var hookRuns atomic.Int32 // the hook registry is process-wide: a repeated run (-count) must not trip over the last one

func TestJoinHooks(t *testing.T) {
	ctx := context.Background()
	run := hookRuns.Add(1)
	pre := fmt.Sprintf("t%d-", run)
	if RegisterJoinHook("", func(context.Context, enroll.JoinResult, string) error { return nil }) == nil || RegisterJoinHook("nameless-fn", nil) == nil {
		t.Fatal("an empty name or a nil function was accepted")
	}
	var mu sync.Mutex
	var order []string
	var gotState string
	var gotNode string
	add := func(name string, err error) {
		t.Helper()
		if e := RegisterJoinHook(pre+name, func(_ context.Context, r enroll.JoinResult, state string) error {
			if hookRuns.Load() != run {
				return nil // a hook of an earlier run
			}
			mu.Lock()
			defer mu.Unlock()
			order, gotState, gotNode = append(order, name), state, r.NodeID
			return err
		}); e != nil {
			t.Fatal(e)
		}
	}
	boom := errors.New("boom")
	add("first", nil)
	add("second", nil)
	if RegisterJoinHook(pre+"first", func(context.Context, enroll.JoinResult, string) error { return nil }) == nil {
		t.Fatal("a duplicate name was accepted")
	}
	if err := RunJoinHooks(ctx, enroll.JoinResult{NodeID: "node-h"}, "/state"); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(order) != "[first second]" || gotState != "/state" || gotNode != "node-h" {
		t.Fatalf("order %v state %q node %q", order, gotState, gotNode)
	}
	add("third", boom)
	add("fourth", nil)
	order = nil
	err := RunJoinHooks(ctx, enroll.JoinResult{}, "/s")
	if !errors.Is(err, boom) || err.Error() != "join hook "+pre+"third: boom" {
		t.Fatalf("err %v", err)
	}
	if fmt.Sprint(order) != "[first second third]" {
		t.Fatalf("the run did not stop at the first error: %v", order)
	}
}

func TestDefaultService(t *testing.T) {
	ctx := context.Background()
	Use(nil)
	t.Cleanup(func() { Use(nil) })
	if _, err := Offer(ctx, OfferOptions{}); !errors.Is(err, ErrNoService) {
		t.Fatalf("Offer: %v", err)
	}
	if err := Withdraw(ctx, "AAAAAAAAAA"); !errors.Is(err, ErrNoService) {
		t.Fatalf("Withdraw: %v", err)
	}
	if err := ServeWhileOffers(ctx, "127.0.0.1:0", "https://127.0.0.1"); !errors.Is(err, ErrNoService) {
		t.Fatalf("ServeWhileOffers: %v", err)
	}
	r := newRig(t, nil)
	Use(r.svc)
	off, err := Offer(ctx, OfferOptions{Approver: approving("policy:d", DefaultTrust()), Intent: "burst"})
	if err != nil {
		t.Fatal(err)
	}
	addr := freeAddr(t)
	cctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- ServeWhileOffers(cctx, addr, "https://127.0.0.1") }()
	eventually(t, time.Second, "the default service opens the listener", func() bool { return isOpen(addr) })
	if err = Withdraw(ctx, off.Code); err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, "withdraw closes it", func() bool { return !isOpen(addr) })
	cancel()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCapabilityDraft(t *testing.T) {
	now := time.Now().UTC()
	full := enroll.Request{Name: "node-c", OS: "linux", Arch: "arm64", MachineID: "abc123", AgentVersion: "1.5.0"}
	for _, q := range []enroll.Request{full, {Name: "node-c", OS: "darwin", Arch: "amd64"}} {
		c := draftCapability("node-c", q, now)
		c.Trust = trustOf(DefaultTrust(), now) // the registry replaces the draft trust; with the approver's it must validate
		if err := model.ValidateCapability(c); err != nil {
			t.Fatalf("draft is not a valid capability: %v", err)
		}
		if c.Identity.ID != "node-c" || c.Identity.Transport != "agent" || *c.Platform.OS.Value != q.OS || *c.Platform.Arch.Value != q.Arch || c.Protocol.AgentVersion != q.AgentVersion {
			t.Fatalf("draft %+v", c.Identity)
		}
		if (q.MachineID == "") != (c.Identity.MachineID.Value == nil) {
			t.Fatalf("machine id fact %+v for %q", c.Identity.MachineID, q.MachineID)
		}
		if c.Identity.MachineID.Value == nil && c.Identity.MachineID.Confidence != "unknown" {
			t.Fatal("an unknown machine id must say so")
		}
		if c.Lifecycle.Eligible || len(c.Lifecycle.AuthorizedProjects) != 0 {
			t.Fatal("a draft authorizes nothing")
		}
	}
	tr := trustOf(TrustFields{Accepts: []string{"owner", "internal"}, SecretClasses: []string{"none"}, PrivacyZone: "local-only"}, now)
	if len(*tr.Accepts.Value) != 2 || tr.Accepts.Source != "assigned" || *tr.PrivacyZone.Value != "local-only" || tr.Isolation.Value != nil {
		t.Fatalf("trust %+v", tr)
	}
	if tr = trustOf(TrustFields{}, now); tr.PrivacyZone.Value != nil || tr.PrivacyZone.Confidence != "unknown" {
		t.Fatalf("an empty zone must stay unknown: %+v", tr.PrivacyZone)
	}
}
