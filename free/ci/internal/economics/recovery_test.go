package economics

import (
	"context"
	"database/sql"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"testing"
)

func TestRecoverySettlesDeadRunAndRetriesInterruptedSettlement(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	c := WithEnv(ctx, envFor(st, spendPolicy(t, paidKV(nil)), clk, "me"))
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a, "", "cloudco", 300_000, 0)); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+dbPathOf(st))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// A prior recovery closed the run but died before settling its ledger.
	if _, err = db.Exec("UPDATE pipeline SET status='lost' WHERE id='pipe'"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE attempt SET state='failed' WHERE id=?", a); err != nil {
		t.Fatal(err)
	}
	// Recovery must use its own store, without a bound economics Env.
	rep := cirun.Recover(ctx, st, "", "")
	if len(rep.Errors) > 0 {
		t.Fatalf("recovery: %v", rep.Errors)
	}
	row, _, err := st.LedgerAttempt(ctx, a)
	if err != nil || row.ActMicro == nil || *row.ActMicro != row.EstMicro || row.Confidence != ConfUnknown {
		t.Fatalf("unsettled: %+v %v", row, err)
	}
	rep = cirun.Recover(ctx, st, "", "")
	again, _, _ := st.LedgerAttempt(ctx, a)
	if len(rep.Errors) > 0 || again.FinalizedAt != row.FinalizedAt {
		t.Fatalf("non-idempotent recovery: %v", rep.Errors)
	}
}

func TestRecoveryClosesAndSettlesRecordedSpan(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	c := WithEnv(ctx, envFor(st, spendPolicy(t, paidKV(nil)), clk, "me"))
	a := queueAttempt(t, st, "dead", "rev", "project")
	if err := place(c, st, request(a, "", "cloudco", 300_000, 0)); err != nil {
		t.Fatal(err)
	}
	coord, err := st.RegisterCoordinator(ctx, 99999999, "gone", "old-boot")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+dbPathOf(st))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := clk.now().UnixNano()
	if _, err = db.Exec("UPDATE attempt SET coordinator_id=?,state='failed',started_at=?,ended_at=? WHERE id=?", coord.ID, start, start+70_000_000_000, a); err != nil {
		t.Fatal(err)
	}
	rep := cirun.Recover(ctx, st, "", "")
	if len(rep.Errors) > 0 {
		t.Fatalf("recovery: %v", rep.Errors)
	}
	pipe, _, _ := st.PipelineByID(ctx, "dead")
	row, _, err := st.LedgerAttempt(ctx, a)
	if pipe.Status != "lost" || err != nil || row.ActMicro == nil || *row.ActMicro != 120_000 || *row.ActMs != 70_000 || row.Confidence != ConfComputed {
		t.Fatalf("status %s ledger %+v err %v", pipe.Status, row, err)
	}
}
