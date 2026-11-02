package store

// Tests of the stats read queries (P7-SCHED-20): an empty store reads clean, rows come back for the
// window and project asked for, and a dropped set is reported missing instead of failing.

import (
	"context"
	"database/sql"
	"strings"
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
	if len(rows.Runs)+len(rows.Jobs)+len(rows.Waits)+len(rows.Leases)+len(rows.Nodes) != 0 || rows.OutboxPending != 0 || rows.Path == "" {
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
	if err != nil || len(all.Runs) != 2 || len(all.Jobs) != 2 {
		t.Fatalf("all: runs %+v, jobs %+v, %v", all.Runs, all.Jobs, err)
	}
	one, err := s.StatsRead(ctx, StatsQuery{Now: now, Since: 24 * time.Hour, Project: "acme/a"})
	if err != nil || len(one.Runs) != 1 || one.Runs[0].N != 1 || one.Runs[0].Project != "acme/a" || len(one.Waits) != 1 || one.Waits[0].Key != "push" {
		t.Fatalf("project a: runs %+v, waits %+v, %v", one.Runs, one.Waits, err)
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

// A text field that holds the separator of the packed outcome row is read through the column form.
func TestStatsReadSeparatorInText(t *testing.T) {
	s := testStore(t)
	now := time.Now()
	statsSeed(t, s, "acme/a", "a1", now.Add(-time.Minute).UnixMilli())
	// stat_row follows job_id, attempt_n and trigger (the triggers that set them rebuild it), so touch one.
	if _, err := s.writer.Exec(`UPDATE schedtel_outcome SET runner='r'||char(31)||'1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writer.Exec(`UPDATE schedtel_outcome SET job_id=job_id`); err != nil {
		t.Fatal(err)
	}
	rows, err := s.StatsRead(context.Background(), StatsQuery{Now: now, Since: time.Hour})
	if err != nil || len(rows.Runs) != 1 || rows.Runs[0].Runner != "r\x1f1" || rows.Runs[0].N != 1 || len(rows.Jobs) != 1 || len(rows.Waits) != 1 {
		t.Fatalf("runs %+v, jobs %+v, waits %+v, %v", rows.Runs, rows.Jobs, rows.Waits, err)
	}
}

// stat_row is what the trigger writes and what the migration's UPDATE backfills for existing rows.
func TestStatsRowTextAndBackfill(t *testing.T) {
	s := testStore(t)
	statsSeed(t, s, "acme/a", "a1", time.Now().UnixMilli())
	const want = "1\x1f10\x1f20\x1f-1\x1f1\x1facme/a\x1fr1\x1fpush\x1fbuild\x1fj-a1"
	read := func() string {
		var got sql.NullString
		if err := s.writer.QueryRow(`SELECT stat_row FROM schedtel_outcome`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got.String
	}
	if got := read(); got != want {
		t.Fatalf("trigger wrote %q, want %q", got, want)
	}
	file, err := schedTelemetryMigrations.ReadFile("migrations/sched-telemetry/0003_outcome_row.sql")
	if err != nil {
		t.Fatal(err)
	}
	var backfill string
	for _, line := range strings.Split(string(file), "\n") {
		if strings.HasPrefix(line, "UPDATE schedtel_outcome SET stat_row=") {
			backfill = line
		}
	}
	if backfill == "" {
		t.Fatal("no backfill statement in the migration")
	}
	if _, err = s.writer.Exec(`UPDATE schedtel_outcome SET stat_row=NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.writer.Exec(backfill); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != want {
		t.Fatalf("backfill wrote %q, want %q", got, want)
	}
}
