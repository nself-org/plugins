package agentapi

// Purpose: the AgentControl reads over the real listener (D-1383): AgentVersion from the last hello, ActiveJobs from the hello and
//          then the last heartbeat (and an error for a node with no session), WaitHello for a hello after a given time.
// Inputs:  the rig. Outputs: pass or fail.
// Constraints: bounded waits.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func TestAgentControlReadsHelloAndHeartbeat(t *testing.T) {
	r := newRig(t, nil)
	ctx := context.Background()
	if _, err := r.srv.AgentVersion(ctx, "n-1"); err == nil {
		t.Fatal("a version for a node that said no hello")
	}
	if _, err := r.srv.ActiveJobs(ctx, "n-1"); err == nil {
		t.Fatal("an idle answer for a node with no session")
	}
	a := r.newAgent("n-1")
	a.connect()
	if v, err := r.srv.AgentVersion(ctx, "n-1"); err != nil || v != "1.5.0" {
		t.Fatalf("AgentVersion = %q %v, want 1.5.0", v, err)
	}
	if n, err := r.srv.ActiveJobs(ctx, "n-1"); err != nil || n != 0 {
		t.Fatalf("ActiveJobs after hello = %d %v, want 0", n, err)
	}
	u := a.openStream(0)
	u.send(protocol.Message{Type: "heartbeat", Body: protocol.Heartbeat{Leases: []protocol.LeaseState{}, Slots: protocol.Slots{Total: 2, Used: 1}}})
	eventually(t, 2*time.Second, "the heartbeat's slot use", func() bool { n, err := r.srv.ActiveJobs(ctx, "n-1"); return err == nil && n == 1 })

	since := time.Now()
	if _, err := r.srv.WaitHello(ctx, "n-1", since, 50*time.Millisecond); err == nil {
		t.Fatal("WaitHello took the hello from before since")
	}
	got := make(chan string, 1)
	go func() { v, _ := r.srv.WaitHello(ctx, "n-1", since, 5*time.Second); got <- v }()
	resp := a.hello(func(h *protocol.Hello) { h.AgentVersion = "1.6.0" })
	_ = resp.Body.Close()
	select {
	case v := <-got:
		if v != "1.6.0" {
			t.Fatalf("WaitHello = %q, want 1.6.0", v)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("WaitHello did not wake on the new hello")
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.srv.WaitHello(cctx, "n-1", time.Now(), time.Minute); err != context.Canceled {
		t.Fatalf("a cancelled wait = %v", err)
	}
}
