package store

// Tests of the stats read queries (P7-SCHED-20): an empty store reads clean, rows come back for the
// window and project asked for, and a dropped set is reported missing instead of failing.

import (
	"context"
	"testing"
	"time"
)

func statsSeed(t *testing.T, s *Store, project, attempt string, tsMs int64) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO pipeline(id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status,created_at) VALUES('p-` + attempt + `','` + project + `','r','push','t','z','d','i','m','r','s','passed',1)`,
		`INSERT INTO job(id,pipeline_id,name,kind,idempotent,infra_max,input_digest,required,status) VALUES('j-` + attempt + `','p-` + attempt + `','build','k',1,1,'i',1,'passed')`,
		`INSERT INTO attempt(id,job_id,n,state) VALUES('` + attempt + `','j-` + attempt + `',1,'passed')`,
	} {
		if _, err := s.writer.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.writer.Exec(`INSERT INTO schedtel_outcome(attempt_id,project,job_key,runner,runner_class,wait_ms,runtime_ms,cold_start_ms,cost_micro,result,failure_class,ts) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		attempt, project, "build", "r1", "owned", 10, 20, -1, 0, "passed", "", tsMs); err != nil {
		t.Fatal(err)
	}
}

func TestStatsReadEmptyStore(t *testing.T) {
	s := testStore(t)
	rows, err := s.StatsRead(context.Background(), StatsQuery{Now: time.Now(), Since: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Outcomes)+len(rows.Leases)+len(rows.Nodes) != 0 || rows.OutboxPending != 0 || rows.Path == "" {
		t.Fatalf("empty store gave rows: %+v", rows)
	}
}

func TestStatsReadWindowAndProject(t *testing.T) {
	s := testStore(t)
	now := time.Now()
	statsSeed(t, s, "acme/a", "a1", now.Add(-time.Minute).UnixMilli())
	statsSeed(t, s, "acme/b", "b1", now.Add(-time.Minute).UnixMilli())
	statsSeed(t, s, "acme/a", "a2", now.Add(-48*time.Hour).UnixMilli()) // outside the window
	ctx := context.Background()
	all, err := s.StatsRead(ctx, StatsQuery{Now: now, Since: 24 * time.Hour})
	if err != nil || len(all.Outcomes) != 2 {
		t.Fatalf("all: %d outcomes, %v", len(all.Outcomes), err)
	}
	one, err := s.StatsRead(ctx, StatsQuery{Now: now, Since: 24 * time.Hour, Project: "acme/a"})
	if err != nil || len(one.Outcomes) != 1 || one.Outcomes[0].Attempt != "a1" || one.Outcomes[0].Trigger != "push" {
		t.Fatalf("project a: %+v, %v", one.Outcomes, err)
	}
}

func TestStatsReadMissingSet(t *testing.T) {
	s := testStore(t)
	if _, err := s.writer.Exec(`DROP TABLE outbox`); err != nil {
		t.Fatal(err)
	}
	rows, err := s.StatsRead(context.Background(), StatsQuery{Now: time.Now(), Since: time.Hour})
	if err != nil || !rows.Missing["outbox"] || rows.Missing["ledger"] {
		t.Fatalf("missing = %v, %v", rows.Missing, err)
	}
}
