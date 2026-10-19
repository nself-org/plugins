package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func telCount(t *testing.T, s *Store, table, project string) int {
	t.Helper()
	var n int
	if err := s.readers.QueryRowContext(context.Background(), `SELECT count(*) FROM `+table+` WHERE project=?`, project).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// telBulk inserts n rows of table for project, ts counting up from first.
func telBulk(t *testing.T, s *Store, table, project string, n int, first int64) {
	t.Helper()
	cols := `(id,run,job,job_name,project,attempt,world_digest,chosen,runner_class,outcome,cost_micro,wait_ms,doc_json,ts) VALUES (?,'r','j','n',?,1,'d','c','k','admitted',0,0,'{}',?)`
	if table == "schedtel_outcome" {
		cols = `(attempt_id,project,job_key,runner,runner_class,wait_ms,runtime_ms,cold_start_ms,cost_micro,result,failure_class,ts) VALUES (?,?,'n','c','k',0,0,0,0,'passed','',?)`
	}
	tx, err := s.writer.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, err = tx.Exec(`INSERT INTO `+table+` `+cols, fmt.Sprintf("%s-%d-%d", project, first, i), project, first+int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestTelemetryStoreRoundTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	admitSeed(t, s, "run1", "proj", "build", "a1")
	a, ok, err := s.TelAttempt(ctx, "a1")
	if err != nil || !ok || a.Run != "run1" || a.JobName != "build" || a.Project != "proj" || a.State != "queued" || a.N != 1 {
		t.Fatalf("attempt: %+v %v %v", a, ok, err)
	}
	if q, err := s.TelQueued(ctx); err != nil || len(q) != 1 || q[0].ID != "a1" {
		t.Fatalf("queued: %+v %v", q, err)
	}
	d := TelDecision{ID: "a1", Run: "run1", Job: a.Job, JobName: "build", Project: "proj", Attempt: 1, WorldDigest: "w", Outcome: "waiting", TS: 100, Doc: json.RawMessage(`{"n":1}`)}
	if err = s.TelPutDecision(ctx, d); err != nil {
		t.Fatal(err)
	}
	d.Outcome, d.Chosen, d.RunnerClass, d.CostMicro, d.WaitMs, d.TS = "admitted", "local", "local", 7, 40, 200
	if err = s.TelPutDecision(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.TelDecision(ctx, "a1")
	if err != nil || !ok || got.Outcome != "admitted" || got.Chosen != "local" || got.WaitMs != 40 || string(got.Doc) != `{"n":1}` {
		t.Fatalf("one record per attempt, latest wins: %+v %v %v", got, ok, err)
	}
	if all, err := s.TelDecisions(ctx, "run1", 0); err != nil || len(all) != 1 {
		t.Fatalf("by run: %+v %v", all, err)
	}
	if _, ok, _ = s.TelDecision(ctx, "nope"); ok {
		t.Fatal("unknown id")
	}
	for i, ms := range []int64{10, 20} {
		o := TelOutcome{AttemptID: fmt.Sprintf("o%d", i), Project: "proj", JobKey: "build", Runner: "local", RunnerClass: "local", Result: "passed", RuntimeMs: ms, TS: int64(300 + i)}
		err = s.TelRecordOutcome(ctx, o, func(cur TelEstimator, found bool) TelEstimator {
			if found != (i > 0) {
				t.Errorf("found=%v at %d", found, i)
			}
			cur.RuntimeMs, cur.N = cur.RuntimeMs+ms, cur.N+1
			return cur
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	est, err := s.TelEstimators(ctx)
	if err != nil || len(est) != 1 || est[0].RuntimeMs != 30 || est[0].N != 2 || est[0].JobKey != "build" {
		t.Fatalf("estimators: %+v %v", est, err)
	}
	if got, err := s.TelRunnerResults(ctx, "local", 20); err != nil || len(got) != 2 {
		t.Fatalf("runner results: %v %v", got, err)
	}
}

func TestTelemetryRetentionBothCases(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	const day = int64(24 * 3600 * 1000)
	now := 1000 * day
	for _, table := range []string{"schedtel_decision", "schedtel_outcome"} {
		// 20 010 rows older than 90 days: the row bound keeps the newest 20 000.
		telBulk(t, s, table, "many-old", TelKeepRows+10, now-100*day)
		// 20 010 rows inside 90 days: the age bound keeps all of them.
		telBulk(t, s, table, "many-new", TelKeepRows+10, now-2*day)
		// few rows, some older than 90 days: the row bound keeps them.
		telBulk(t, s, table, "few", 30, now-100*day)
		telBulk(t, s, table, "few", 10, now-day)
		for _, p := range []string{"many-old", "many-new", "few"} {
			tx, err := s.writer.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if err = telPrune(ctx, tx, table, p, now, TelKeepMs, TelKeepRows); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
		}
		for p, want := range map[string]int{"many-old": TelKeepRows, "many-new": TelKeepRows + 10, "few": 40} {
			if got := telCount(t, s, table, p); got != want {
				t.Errorf("%s/%s kept %d, want %d", table, p, got, want)
			}
		}
	}
	if TelKeepMs != 90*day || TelKeepRows != 20000 {
		t.Fatalf("retention bounds: %d ms, %d rows", TelKeepMs, TelKeepRows)
	}
	// the same pruning runs inside the insert transaction of the real writers
	admitSeed(t, s, "run2", "few", "build", "x1")
	if err := s.TelPutDecision(ctx, TelDecision{ID: "x1", Run: "run2", Job: "job-x1", JobName: "build", Project: "few", Outcome: "waiting", TS: now, Doc: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if got := telCount(t, s, "schedtel_decision", "few"); got != 41 {
		t.Fatalf("insert keeps what retention keeps: %d", got)
	}
}
