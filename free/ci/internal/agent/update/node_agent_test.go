package update

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

type resultCtl struct {
	AgentControl
	sent   []bool
	result protocol.UpgradeResult
	err    error
}

func (c *resultCtl) Upgrade(_, _, _ string, allowDowngrade bool) error {
	c.sent = append(c.sent, allowDowngrade)
	return nil
}

func (c *resultCtl) WaitUpgradeResult(context.Context, string, string, time.Duration) (protocol.UpgradeResult, error) {
	return c.result, c.err
}

func TestAgentNodeInstallWaitsForTheAgentsAnswer(t *testing.T) {
	in := InstallInput{Version: "1.4.0", ManifestURL: "https://x/m.json", AllowDowngrade: true}
	t.Run("a refusal is E666 with the agent's reason", func(t *testing.T) {
		c := &resultCtl{result: protocol.UpgradeResult{Version: "1.4.0", Status: protocol.UpgradeRefused, Reason: ReasonCeiling}}
		err := (&AgentNode{NodeID: "n1", Ctl: c}).Install(context.Background(), in)
		if ReasonOf(err) != ReasonCeiling || !strings.HasPrefix(err.Error(), "E666:") || len(c.sent) != 1 || !c.sent[0] {
			t.Fatalf("err %v sent %v", err, c.sent)
		}
	})
	t.Run("done is success", func(t *testing.T) {
		c := &resultCtl{result: protocol.UpgradeResult{Version: "1.4.0", Status: protocol.UpgradeDone}}
		if err := (&AgentNode{NodeID: "n1", Ctl: c}).Install(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("no answer leaves the verdict to the hello", func(t *testing.T) {
		c := &resultCtl{err: errors.New("no upgrade result")}
		if err := (&AgentNode{NodeID: "n1", Ctl: c}).Install(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a cancelled run stops", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c := &resultCtl{err: ctx.Err()}
		if err := (&AgentNode{NodeID: "n1", Ctl: c}).Install(ctx, in); !errors.Is(err, context.Canceled) {
			t.Fatalf("err %v", err)
		}
	})
	t.Run("a pull node may now be downgraded", func(t *testing.T) {
		if d, ok := any(&AgentNode{}).(Downgrader); ok && !d.CanDowngrade() {
			t.Fatal("the upgrade message carries allow_downgrade; the node must not refuse a downgrade up front")
		}
	})
}
