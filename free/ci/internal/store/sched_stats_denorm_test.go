package store

// Tests of the write-time stats columns (P7-SCHED-20): an outcome reads its job, attempt number and trigger
// whichever of outcome and attempt is written first and drops out when its attempt is pruned, as the old
// attempt-job-pipeline join did; a cache use follows its cache_attempt project the same way.

import (
	"context"
	"testing"
	"time"
)

func TestStatsOutcomeJobFollowsAttempt(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.writer.Exec(q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	read := func() StatsRows {
		t.Helper()
		rows, err := s.StatsRead(ctx, StatsQuery{Now: now, Since: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	exec(`INSERT INTO schedtel_outcome(attempt_id,project,job_key,runner,runner_class,wait_ms,runtime_ms,cold_start_ms,cost_micro,result,failure_class,ts) VALUES('a1','acme/a','build','r1','owned',10,20,-1,0,'passed','',?)`, now.UnixMilli())
	exec(`INSERT INTO cache_use(attempt_id,spec,cache_key,result,bytes,ms,ts) VALUES('a1','go','k','hit',7,1,?)`, now.UnixNano())
	if rows := read(); len(rows.Outcomes) != 0 || len(rows.Cache) != 1 || rows.Cache[0].Project != "" {
		t.Fatalf("before the attempt: outcomes %+v, cache %+v", rows.Outcomes, rows.Cache)
	}
	exec(`INSERT INTO pipeline(id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status,created_at) VALUES('p1','acme/a','r','schedule','t','z','d','i','m','r','s','passed',1)`)
	exec(`INSERT INTO job(id,pipeline_id,name,kind,idempotent,infra_max,input_digest,required,status) VALUES('j1','p1','build','k',1,1,'i',1,'passed')`)
	exec(`INSERT INTO attempt(id,job_id,n,state) VALUES('a1','j1',2,'passed')`)
	exec(`INSERT INTO cache_attempt(attempt_id,project,trust,protection,secrets_declared,private_deps,recorded_at) VALUES('a1','acme/a','owner','none',0,0,1)`)
	rows := read()
	if len(rows.Outcomes) != 1 || rows.Outcomes[0].Job != "j1" || rows.Outcomes[0].N != 2 || rows.Outcomes[0].Trigger != "schedule" {
		t.Fatalf("after the attempt: %+v", rows.Outcomes)
	}
	if len(rows.Cache) != 1 || rows.Cache[0].Project != "acme/a" || rows.Cache[0].Sum != 7 {
		t.Fatalf("cache after its attempt: %+v", rows.Cache)
	}
	exec(`DELETE FROM cache_attempt WHERE attempt_id='a1'`)
	exec(`DELETE FROM attempt WHERE id='a1'`)
	if rows = read(); len(rows.Outcomes) != 0 || len(rows.Cache) != 1 || rows.Cache[0].Project != "" {
		t.Fatalf("after the prune: outcomes %+v, cache %+v", rows.Outcomes, rows.Cache)
	}
}
