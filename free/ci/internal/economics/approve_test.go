package economics

// Tests of spend approval (Epic D11, X4): the threshold holds a pipeline in
// approval.pending until an admin approves it; the approval is bound to the
// run, head SHA, config digest and estimate and is audited.

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type approvalRig struct {
	ctx      context.Context
	st       *store.Store
	clk      *clock
	svc      *roles.Service
	env      context.Context
	admin    roles.Actor
	member   roles.Actor
	operator roles.Actor
}

func newApprovalRig(t *testing.T) *approvalRig {
	t.Helper()
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	svc := roles.New(st, &audit.Log{St: st})
	svc.Now = clk.now
	svc.Self = "alice"
	op := svc.Operator("cli:alice")
	token := func(principal string, role roles.Role) roles.Actor {
		tok, _, err := svc.IssueToken(ctx, op, roles.Request{Principal: principal, Role: role, Projects: []string{roles.AllProjects}})
		if err != nil {
			t.Fatal(err)
		}
		a, err := svc.ActorFromToken(ctx, tok)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	p := spendPolicy(t, paidKV(map[string]string{schedpolicy.KeyApproval: `100000`}))
	return &approvalRig{ctx: ctx, st: st, clk: clk, svc: svc, operator: op, admin: token("adm", roles.Admin), member: token("mem", roles.Member), env: WithEnv(ctx, envFor(st, p, clk, "me"))}
}

func TestBudgetApprovalHoldsAboveTheThreshold(t *testing.T) {
	r := newApprovalRig(t)
	a := queueAttempt(t, r.st, "pipe", "sha-1", "project")
	small := queueAttempt(t, r.st, "pipe", "sha-1", "project")
	if err := place(r.env, r.st, request(small, "", "cloudco", 60_000, 0)); err != nil { // 60 000 <= 100 000
		t.Fatalf("under the threshold needs no approval: %v", err)
	}
	ref := refusalOf(t, place(r.env, r.st, request(a, "", "cloudco", 300_000, 0)))
	if ref.Reason != model.ApprovalPending || !strings.HasPrefix(ref.Detail, PendingDetailPrefix+"360000") {
		t.Fatalf("above the threshold: %+v", ref)
	}
	if _, err := ApproveSpend(r.env, "pipe", r.member); err == nil || roles.ReasonOf(err) != roles.ReasonRole {
		t.Fatalf("a member must be refused (E687 role), got %v", err)
	}
	if ap, _ := r.st.Approvals(r.ctx, 10); len(ap) != 0 {
		t.Fatalf("a refused approval wrote a row: %+v", ap)
	}
	got, err := ApproveSpend(r.env, "pipe", r.admin)
	if err != nil {
		t.Fatal(err)
	}
	if got.EstimateMicro != 360_000 || got.SHA != "sha-1" || got.ConfigDigest != "cfg1" || got.Run != "pipe" || !strings.Contains(got.Actor, "adm") {
		t.Fatalf("approval: %+v", got)
	}
	var found bool
	entries, _ := r.st.AuditEntries(r.ctx, 0, 0)
	for _, e := range entries {
		if e.Action == "spend.approved" && e.Target == "pipe" && strings.Contains(e.Detail, "amount_micro=360000") && strings.Contains(e.Detail, "sha=sha-1") && strings.Contains(e.Detail, "config=cfg1") && strings.Contains(e.Actor, "adm") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the approval is not audited with actor, amount, SHA and digest: %+v", entries)
	}
	if err = place(r.env, r.st, request(a, "", "cloudco", 300_000, 0)); err != nil {
		t.Fatalf("after approval: %v", err)
	}
	// A job that takes the pipeline past the approved estimate is held again.
	b := queueAttempt(t, r.st, "pipe", "sha-1", "project")
	if ref = refusalOf(t, place(r.env, r.st, request(b, "", "cloudco", 60_000, 0))); ref.Reason != model.ApprovalPending || !strings.HasPrefix(ref.Detail, PendingDetailPrefix+"420000") {
		t.Fatalf("a pipeline past what was approved: %+v", ref)
	}
}

func TestBudgetPersonalOperatorMayApprove(t *testing.T) {
	r := newApprovalRig(t)
	a := queueAttempt(t, r.st, "pipe", "sha-1", "project")
	_ = place(r.env, r.st, request(a, "", "cloudco", 300_000, 0))
	if _, err := ApproveSpend(r.env, "pipe", r.operator); err != nil {
		t.Fatalf("the local operator holds admin: %v", err)
	}
}

func TestBudgetNewPushVoidsTheApproval(t *testing.T) {
	r := newApprovalRig(t)
	a := queueAttempt(t, r.st, "pipe", "sha-1", "project")
	_ = place(r.env, r.st, request(a, "", "cloudco", 300_000, 0))
	if _, err := ApproveSpend(r.env, "pipe", r.admin); err != nil {
		t.Fatal(err)
	}
	if valid, _ := r.st.ValidApprovals(r.ctx); valid["pipe"] != 300_000 {
		t.Fatalf("valid approvals: %v", valid)
	}
	// A new push is a new run with its own SHA: the old approval does not carry over.
	b := queueAttempt(t, r.st, "pipe2", "sha-2", "project")
	if ref := refusalOf(t, place(r.env, r.st, request(b, "", "cloudco", 300_000, 0))); ref.Reason != model.ApprovalPending {
		t.Fatalf("a new run needs its own approval: %+v", ref)
	}
	// The same run whose head SHA or config digest changed under it loses the approval too.
	for _, col := range []string{"revision", "input_digest"} {
		db, err := sql.Open("sqlite", "file:"+dbPathOf(r.st)+"?_pragma=busy_timeout(5000)")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("UPDATE pipeline SET " + col + "='changed' WHERE id='pipe'"); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
		if valid, _ := r.st.ValidApprovals(r.ctx); len(valid) != 0 {
			t.Fatalf("a changed %s must void the approval: %v", col, valid)
		}
		// The admission step must see the void too, not only the world's view.
		retry := queueAttempt(t, r.st, "pipe", "sha-1", "project")
		if ref := refusalOf(t, place(r.env, r.st, request(retry, "", "cloudco", 300_000, 0))); ref.Reason != model.ApprovalPending {
			t.Fatalf("a changed %s must hold the job again at admission: %+v", col, ref)
		}
		restore, _ := sql.Open("sqlite", "file:"+dbPathOf(r.st)+"?_pragma=busy_timeout(5000)")
		if col == "revision" {
			_, _ = restore.Exec("UPDATE pipeline SET revision='sha-1' WHERE id='pipe'")
		} else {
			_, _ = restore.Exec("UPDATE pipeline SET input_digest='cfg1' WHERE id='pipe'")
		}
		_ = restore.Close()
		if valid, _ := r.st.ValidApprovals(r.ctx); valid["pipe"] != 300_000 {
			t.Fatalf("restoring the SHA and digest restores the approval: %v", valid)
		}
	}
}

func TestBudgetApproveSpendNeedsSomethingToApprove(t *testing.T) {
	r := newApprovalRig(t)
	queueAttempt(t, r.st, "pipe", "sha-1", "project")
	if _, err := ApproveSpend(r.env, "pipe", r.admin); err == nil || !strings.Contains(err.Error(), "E695") {
		t.Fatalf("nothing awaits approval: %v", err)
	}
	if _, err := ApproveSpend(r.env, "no-such-run", r.admin); err == nil {
		t.Fatal("approved an unknown run")
	}
	if err := NotePending(r.env, "pipe", 250_000); err != nil {
		t.Fatal(err)
	}
	if err := NotePending(r.env, "pipe", 100_000); err != nil { // a smaller later figure never lowers it
		t.Fatal(err)
	}
	got, err := ApproveSpend(r.env, "pipe", r.admin)
	if err != nil || got.EstimateMicro != 250_000 {
		t.Fatalf("noted estimate: %+v %v", got, err)
	}
}
