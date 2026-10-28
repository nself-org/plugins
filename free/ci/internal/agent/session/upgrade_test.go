package session

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func upgradeRig(t *testing.T, hook func(context.Context, protocol.Upgrade) (string, error)) *rig {
	t.Helper()
	r := startRig(t, okRunner(`{}`), func(c *Config) { c.Upgrade = hook })
	r.handshake()
	return r
}

func TestUpgradeMessageIsAnsweredWithAResultFrame(t *testing.T) {
	var got atomic.Value
	for name, tc := range map[string]struct {
		reason string
		want   protocol.UpgradeResult
	}{
		"done":    {"", protocol.UpgradeResult{Version: "1.5.0", Status: protocol.UpgradeDone}},
		"refused": {"running_job", protocol.UpgradeResult{Version: "1.5.0", Status: protocol.UpgradeRefused, Reason: "running_job"}},
	} {
		t.Run(name, func(t *testing.T) {
			r := upgradeRig(t, func(_ context.Context, u protocol.Upgrade) (string, error) { got.Store(u); return tc.reason, nil })
			r.car.push("upgrade", protocol.Upgrade{Version: "1.5.0", ManifestURL: "https://x/m.json", AllowDowngrade: true})
			if res := r.car.next(t, "upgrade-result").Body.(protocol.UpgradeResult); res != tc.want {
				t.Fatalf("result %+v, want %+v", res, tc.want)
			}
			if u := got.Load().(protocol.Upgrade); u.Version != "1.5.0" || !u.AllowDowngrade {
				t.Fatalf("hook saw %+v", u)
			}
		})
	}
}

func TestUpgradeFailureSendsNoResultAndWithoutAHookIsIgnored(t *testing.T) {
	failing := upgradeRig(t, func(context.Context, protocol.Upgrade) (string, error) { return "", errors.New("E656: boom") })
	failing.car.push("upgrade", protocol.Upgrade{Version: "1.5.0", ManifestURL: "https://x/m.json"})
	none := upgradeRig(t, nil)
	none.car.push("upgrade", protocol.Upgrade{Version: "1.5.0", ManifestURL: "https://x/m.json"})
	time.Sleep(100 * time.Millisecond)
	for name, r := range map[string]*rig{"failing": failing, "no hook": none} {
		for _, m := range r.car.drainOut() {
			if m.Type == "upgrade-result" {
				t.Fatalf("%s: sent %+v", name, m.Body)
			}
		}
	}
}

func TestUpgradeDoesNotBlockHeartbeats(t *testing.T) {
	release := make(chan struct{})
	r := upgradeRig(t, func(ctx context.Context, _ protocol.Upgrade) (string, error) {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return "", nil
	})
	r.car.push("upgrade", protocol.Upgrade{Version: "1.5.0", ManifestURL: "https://x/m.json"})
	r.clock.advance(10 * time.Second)
	r.car.next(t, "heartbeat") // the session answers while the upgrade is still running
	close(release)
	r.car.next(t, "upgrade-result")
}
