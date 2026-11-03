package main

// Purpose: the agent's answer to the coordinator's protocol `upgrade` frame (D-1383). upgradeExit is the session's Upgrade hook:
//          it runs the same verified swap as `nself-ci-agent upgrade` (update.ApplyLocal, with AllowRestore so a rollback to the
//          kept .prev binary needs no downgrade flag), and, as the session's carrier, ends the session once the `done`
//          upgrade-result of a successful apply is on the wire, so the pull loop exits 0 and the service manager starts the new binary.
// Inputs:  the frame (version, manifest URL, allow_downgrade); the state dir; the running binary (os.Executable).
// Outputs: the E666 reason key for a refusal, an error for any other failure (bad signature or digest, a version run that does
//          not match), nil after a swap or when the version already runs.
// Constraints: a refusal or a failure leaves this process running on its current binary and the session open; a frame for the
//          version that already runs answers done and ends the session too (the coordinator waits for a hello after every
//          done, and a restart on the same binary is harmless); the manifest URL must name this platform's
//          manifest, as for the command; nothing here restarts a process.

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"

	"github.com/nself-org/plugins/free/ci/internal/agent/install"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/update"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// upgradeApply is the swap; tests replace it.
var upgradeApply = update.ApplyLocal

// upgradeExit wraps a session's carrier and serves as its Upgrade hook.
type upgradeExit struct {
	session.Carrier
	state   string
	logf    func(string, ...any)
	cancel  context.CancelFunc // ends the session's context
	swapped atomic.Bool        // the hook applied the frame (installed, restored, or already running the version)
	exited  atomic.Bool        // the done result of that swap was sent and the session was told to end
}

func newUpgradeExit(car session.Carrier, state string, cancel context.CancelFunc, logf func(string, ...any)) *upgradeExit {
	return &upgradeExit{Carrier: car, state: state, cancel: cancel, logf: logf}
}

// Send passes the frame on; a sent `done` upgrade-result after a successful apply ends the session.
func (u *upgradeExit) Send(m protocol.Message) error {
	err := u.Carrier.Send(m)
	if err == nil && m.Type == "upgrade-result" && u.swapped.Load() && resultDone(m.Body) {
		u.exited.Store(true)
		u.logf("upgrade installed; exiting so the service manager starts the new binary")
		u.cancel()
	}
	return err
}

func resultDone(body any) bool {
	switch r := body.(type) {
	case protocol.UpgradeResult:
		return r.Status == protocol.UpgradeDone
	case *protocol.UpgradeResult:
		return r != nil && r.Status == protocol.UpgradeDone
	}
	return false
}

// upgraded reports that the session ended because this agent replaced itself.
func (u *upgradeExit) upgraded() bool { return u.exited.Load() }

// Hook is session.Config.Upgrade.
func (u *upgradeExit) Hook(ctx context.Context, f protocol.Upgrade) (string, error) {
	res, err := u.apply(ctx, f)
	if err != nil {
		u.logf("upgrade to %s not installed: %v", f.Version, err)
		return update.ReasonOf(err), err
	}
	if res.Noop {
		u.logf("upgrade to %s: already running it", f.Version)
	}
	u.swapped.Store(true)
	return "", nil
}

func (u *upgradeExit) apply(ctx context.Context, f protocol.Upgrade) (update.Result, error) {
	if !verify.ValidVersion(f.Version) {
		return update.Result{}, fmt.Errorf("E651: the upgrade frame names no valid version (%q)", f.Version)
	}
	base, err := manifestBase(f.ManifestURL)
	if err != nil {
		return update.Result{}, err
	}
	exe, err := agentBinary()
	if err != nil {
		return update.Result{}, fmt.Errorf("E656: agent binary: %w", err)
	}
	v, err := upgradeVerifier()
	if err != nil {
		return update.Result{}, err
	}
	return upgradeApply(ctx, update.LocalOptions{
		StateDir: u.state, BinPath: exe, Current: Version, Version: f.Version,
		AllowDowngrade: f.AllowDowngrade, AllowRestore: true,
		Platform:   lifecycle.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Source:     update.ReleaseSource{Release: install.Release{Version: f.Version, BaseURL: base, Verifier: v}},
		RunVersion: upgradeRunVersion,
	})
}
