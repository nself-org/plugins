//go:build scenarios

package scenarios

// Purpose: Epic acceptance 6, lifecycle bullets: provision -> join -> run -> teardown on a TEAM
// coordinator ends with zero labelled resources and the node revoked; a VM runs exactly ONE job and
// its node certificate is revoked at teardown; an idle VM is torn down at IdleTimeout.
// Inputs:  burst.Engine over fakecloud, a real store, the real dispatch.Admit with the `ephemeral` step.
// Outputs: assertions only.
// Constraints: no sleeps; the injected fakecloud.Clock moves time.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func burstRefusal(t *testing.T, err error) *dispatch.Refusal {
	t.Helper()
	var ref *dispatch.Refusal
	if !errors.As(err, &ref) {
		t.Fatalf("want an admission refusal, got %v", err)
	}
	return ref
}

func TestBurstFullLifecycleLeavesNothingBehind(t *testing.T) {
	ctx := context.Background()
	r := burstNew(t)
	res := r.provision("job-1")
	if len(r.labelled(burstCoord)) != 1 || res.Resource.ID == "" {
		t.Fatalf("provision must create one labelled machine: %+v", r.labelled(burstCoord))
	}
	r.join(res.Token, "node-1") // the machine boots and joins as an ephemeral runner
	a := queueAttempt(t, r.st, "pipe-life", "project", "build")
	if err := burstAdmit(ctx, r.st, a, "node-1", "owner"); err != nil {
		t.Fatalf("the job on the joined node: %v", err)
	}
	r.clk.Advance(10 * time.Minute)
	if _, err := r.eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.cloud.All()) != 1 || r.intent(res.Token).State != store.BurstLive {
		t.Fatal("a node with a leased job must be left alone")
	}
	pass(t, r.st, a) // the job finishes
	if _, err := r.eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if left := r.labelled(burstCoord); len(left) != 0 || len(r.cloud.All()) != 0 {
		t.Fatalf("labelled resources after teardown: %+v", left)
	}
	if len(r.revoked) != 1 || r.revoked[0] != "node-1" {
		t.Fatalf("the node must be revoked once: %v", r.revoked)
	}
	if n, err := r.st.NodeGet(ctx, "node-1"); err != nil || n.State != "revoked" {
		t.Fatalf("node %+v %v", n, err)
	}
	if in, row := r.intent(res.Token), r.resourceRow(res.Token); in.State != store.BurstTerminal || row.ConfirmedTeardownAt == nil {
		t.Fatalf("intent %+v ledger %+v", in, row)
	}
	if len(r.alerts) != 0 {
		t.Fatalf("alerts on a clean lifecycle: %v", r.alerts)
	}
}

func TestBurstOneJobPerVmAndCertRevokedAtTeardown(t *testing.T) {
	ctx := context.Background()
	for _, second := range []model.TrustClass{"owner", "internal", "collaborator", "untrusted"} {
		t.Run(string(second), func(t *testing.T) {
			r := burstNew(t)
			res := r.provision("job-1")
			r.join(res.Token, "node-1")
			a1, a2 := queueAttempt(t, r.st, "pipe-one", "project", "one"), queueAttempt(t, r.st, "pipe-one", "project", "two")
			if err := burstAdmit(ctx, r.st, a1, "node-1", "owner"); err != nil {
				t.Fatalf("the first job: %v", err)
			}
			ref := burstRefusal(t, burstAdmit(ctx, r.st, a2, "node-1", second))
			if ref.Reason != model.EphemeralUsed {
				t.Fatalf("a second job on a used node: %s (%s), want ephemeral.used", ref.Reason, ref.Detail)
			}
			if state, _, _ := r.st.AttemptStateEpoch(ctx, a2); state != "queued" {
				t.Fatalf("the refused attempt stays queued, got %s", state)
			}
			if len(r.revoked) != 0 {
				t.Fatal("the node is revoked only at teardown, not while its job runs")
			}
			pass(t, r.st, a1)
			if _, err := r.eng.Tick(ctx); err != nil {
				t.Fatal(err)
			}
			n, err := r.st.NodeGet(ctx, "node-1")
			if err != nil || n.State != "revoked" || len(r.revoked) != 1 {
				t.Fatalf("node cert revoked at teardown: node %+v err %v revoked %v", n, err, r.revoked)
			}
			// a late retry of the second attempt cannot reuse the torn-down node either
			if ref = burstRefusal(t, burstAdmit(ctx, r.st, a2, "node-1", second)); ref.Reason != model.EphemeralUsed {
				t.Fatalf("a torn-down node took a second job: %s", ref.Reason)
			}
		})
	}
}

func TestBurstIdleVmIsTornDownAtIdleTimeout(t *testing.T) {
	ctx := context.Background()
	r := burstNew(t)
	res := r.provision("job-1")
	r.join(res.Token, "node-1")
	r.clk.Advance(r.cfg.IdleTimeout - time.Minute)
	if _, err := r.eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.cloud.All()) != 1 || len(r.revoked) != 0 {
		t.Fatal("torn down before the idle timeout")
	}
	r.clk.Advance(2 * time.Minute) // now past join + IdleTimeout
	if _, err := r.eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.cloud.All()) != 0 || len(r.revoked) != 1 || r.intent(res.Token).State != store.BurstTerminal {
		t.Fatalf("an idle node must be revoked and deleted: machines %d revoked %v", len(r.cloud.All()), r.revoked)
	}
	if row := r.resourceRow(res.Token); row.ConfirmedTeardownAt == nil {
		t.Fatal("the reservation is released once the idle machine is gone")
	}
}
