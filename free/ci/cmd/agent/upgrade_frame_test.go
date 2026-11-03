package main

// Purpose: D-1383, agent side: a running `pull` agent hands the coordinator's `upgrade` frame to update.ApplyLocal (with
//          AllowRestore and the frame's allow_downgrade), sends its upgrade-result before it exits 0 after a swap, and stays
//          connected and running after a refusal or a failed verification.
// Inputs:  the loopback pull rig (real agent listener, real `pull` command); upgradeApply replaced so nothing is downloaded
//          or swapped. Outputs: pass or fail.
// Constraints: bounded waits; the test binary is never replaced.

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/update"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// fakeApply replaces the swap with fn and records every LocalOptions it is called with.
func fakeApply(t *testing.T, fn func(update.LocalOptions) (update.Result, error)) func() []update.LocalOptions {
	t.Helper()
	var mu sync.Mutex
	var calls []update.LocalOptions
	oldApply, oldVerifier := upgradeApply, upgradeVerifier
	upgradeApply = func(_ context.Context, o update.LocalOptions) (update.Result, error) {
		mu.Lock()
		calls = append(calls, o)
		mu.Unlock()
		return fn(o)
	}
	upgradeVerifier = func() (*verify.Verifier, error) { return &verify.Verifier{}, nil }
	t.Cleanup(func() { upgradeApply, upgradeVerifier = oldApply, oldVerifier })
	return func() []update.LocalOptions {
		mu.Lock()
		defer mu.Unlock()
		return append([]update.LocalOptions{}, calls...)
	}
}

func frameManifestURL(version string) string {
	return "https://example.invalid/v" + version + "/nself-ci-agent-" + verify.Target(runtime.GOOS, runtime.GOARCH) + ".manifest.json"
}

// sendUpgrade queues the frame once the agent's control stream is open.
func sendUpgrade(t *testing.T, r *pullRig, version string, allowDowngrade bool) {
	t.Helper()
	pullWait(t, 15*time.Second, "the control stream", func() bool {
		err := r.srv.Upgrade("n-1", version, frameManifestURL(version), allowDowngrade)
		if err != nil && !errors.Is(err, agentapi.ErrNoControlStream) {
			t.Fatalf("Upgrade: %v", err)
		}
		return err == nil
	})
}

func TestPullAgentAppliesTheUpgradeFrameAnswersAndExits(t *testing.T) {
	state := useState(t)
	r := newPullRig(t)
	r.enroll(state, "n-1", r.base)
	calls := fakeApply(t, func(o update.LocalOptions) (update.Result, error) {
		return update.Result{From: o.Current, To: o.Version}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	code, errb := startPull(t, ctx)
	pullWait(t, 15*time.Second, "the agent's hello", func() bool { v, err := r.srv.AgentVersion(ctx, "n-1"); return err == nil && v == Version })

	sendUpgrade(t, r, "1.6.0", true)
	res, err := r.srv.WaitUpgradeResult(ctx, "n-1", "1.6.0", 15*time.Second)
	if err != nil || res.Status != protocol.UpgradeDone {
		t.Fatalf("upgrade-result %+v (%v), want done\n%s", res, err, errb.String())
	}
	select {
	case c := <-code:
		if c != 0 {
			t.Fatalf("pull exited %d after the upgrade\n%s", c, errb.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("pull kept running after a done upgrade\n%s", errb.String())
	}
	got := calls()
	if len(got) != 1 {
		t.Fatalf("ApplyLocal ran %d times", len(got))
	}
	o := got[0]
	if o.Version != "1.6.0" || o.Current != Version || !o.AllowRestore || !o.AllowDowngrade || o.StateDir != state || o.BinPath == "" || o.RunVersion == nil {
		t.Fatalf("ApplyLocal options %+v", o)
	}
}

func TestPullAgentKeepsRunningWhenTheUpgradeIsRefusedOrFailsVerification(t *testing.T) {
	state := useState(t)
	r := newPullRig(t)
	r.enroll(state, "n-1", r.base)
	fakeApply(t, func(o update.LocalOptions) (update.Result, error) {
		if o.Version == "1.4.0" {
			return update.Result{}, &update.Error{Reason: update.ReasonDowngrade, Msg: "1.4.0 is lower than this agent"}
		}
		return update.Result{}, errors.New("E655: the release signature does not verify")
	})
	ctx, cancel := context.WithCancel(context.Background())
	code, errb := startPull(t, ctx)
	pullWait(t, 15*time.Second, "the agent to connect", func() bool { return len(r.srv.NodesConnected()) == 1 })

	sendUpgrade(t, r, "1.4.0", false)
	res, err := r.srv.WaitUpgradeResult(ctx, "n-1", "1.4.0", 15*time.Second)
	if err != nil || res.Status != protocol.UpgradeRefused || res.Reason != update.ReasonDowngrade {
		t.Fatalf("upgrade-result %+v (%v), want refused downgrade\n%s", res, err, errb.String())
	}
	sendUpgrade(t, r, "1.7.0", false)
	if res, err = r.srv.WaitUpgradeResult(ctx, "n-1", "1.7.0", time.Second); err == nil {
		t.Fatalf("a failed verification answered %+v", res)
	}
	select {
	case c := <-code:
		t.Fatalf("pull exited %d after a refused upgrade\n%s", c, errb.String())
	case <-time.After(500 * time.Millisecond):
	}
	if got := r.srv.NodesConnected(); len(got) != 1 {
		t.Fatalf("NodesConnected after the refusals = %v", got)
	}
	cancel()
	select {
	case c := <-code:
		if c != 0 {
			t.Fatalf("pull exited %d\n%s", c, errb.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("pull did not stop")
	}
}
