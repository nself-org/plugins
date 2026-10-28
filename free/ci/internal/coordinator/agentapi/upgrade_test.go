package agentapi

// Purpose: the upgrade conversation over the real listener: the upgrade frame reaches the agent with allow_downgrade, and the
//          agent's upgrade-result frame comes back to WaitUpgradeResult (done and refused with its reason key).
// Inputs:  the rig. Outputs: pass or fail.
// Constraints: an answer for another version or from before the last Upgrade is never returned.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func TestUpgradeFrameAndResult(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	a.connect()
	resp, err := a.cl.Do(a.req(http.MethodGet, "/v1/agent/control", nil, nil))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("control: %v %v", resp, err)
	}
	defer resp.Body.Close()
	fr := newFrameReader(resp.Body)
	u := a.openStream(0)
	ctx := context.Background()

	for _, tc := range []struct {
		version string
		down    bool
		answer  protocol.UpgradeResult
	}{
		{"1.5.0", false, protocol.UpgradeResult{Version: "1.5.0", Status: protocol.UpgradeDone}},
		{"1.4.0", true, protocol.UpgradeResult{Version: "1.4.0", Status: protocol.UpgradeRefused, Reason: "running_job"}},
	} {
		eventually(t, 2*time.Second, "the control stream to count as live", func() bool {
			return r.srv.Upgrade("n-1", tc.version, "https://x/nself-ci-agent-linux-amd64.manifest.json", tc.down) == nil
		})
		m, err := fr.next()
		if err != nil || m.Type != "upgrade" {
			t.Fatalf("control frame = %+v %v, want upgrade", m, err)
		}
		if up := m.Body.(*protocol.Upgrade); up.Version != tc.version || up.AllowDowngrade != tc.down {
			t.Fatalf("upgrade = %+v, want %s down=%v", up, tc.version, tc.down)
		}
		if _, err = r.srv.WaitUpgradeResult(ctx, "n-1", tc.version, 50*time.Millisecond); err == nil {
			t.Fatal("a result came back before the agent sent one")
		}
		u.send(protocol.Message{Type: "upgrade-result", Body: tc.answer})
		got, err := r.srv.WaitUpgradeResult(ctx, "n-1", tc.version, 5*time.Second)
		if err != nil || got != tc.answer {
			t.Fatalf("result = %+v %v, want %+v", got, err, tc.answer)
		}
	}
}

func TestUpgradeResultIsForTheVersionAsked(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	a.connect()
	resp, err := a.cl.Do(a.req(http.MethodGet, "/v1/agent/control", nil, nil))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("control: %v %v", resp, err)
	}
	defer resp.Body.Close()
	u := a.openStream(0)
	u.send(protocol.Message{Type: "upgrade-result", Body: protocol.UpgradeResult{Version: "1.5.0", Status: protocol.UpgradeDone}})
	if _, err = r.srv.WaitUpgradeResult(context.Background(), "n-1", "1.5.0", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	// a new Upgrade forgets the earlier answer, and an answer for another version is not this one
	eventually(t, 2*time.Second, "the control stream to count as live", func() bool {
		return r.srv.Upgrade("n-1", "1.6.0", "https://x/m.json", false) == nil
	})
	if _, err = r.srv.WaitUpgradeResult(context.Background(), "n-1", "1.6.0", 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), "no upgrade result") {
		t.Fatalf("an old answer was taken for the new upgrade: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = r.srv.WaitUpgradeResult(ctx, "n-1", "1.6.0", time.Minute); err != context.Canceled {
		t.Fatalf("a cancelled wait = %v", err)
	}
}
