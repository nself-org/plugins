package stats

// Purpose: TestStatsQueryPlan (P7-SCHED-20): on a 100 000-attempt fixture every stats statement is planned
// through an index (no full table scan), and the whole document is built inside the read budget.
// Inputs:  buildFixture(t, 80000): 80 000 jobs and 100 000 attempts.
// Outputs: pass or fail.
// Constraints: the budget is 500 ms of CPU time on the Linux runner and ten times that elsewhere, where the machine
//          is shared and the file system slower, and ten times again under the race detector; the statements are the ones internal/store runs.

import (
	"context"
	"database/sql"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestStatsQueryPlan(t *testing.T) {
	st := buildFixture(t, 80000)
	ctx := context.Background()
	q := store.StatsQuery{Now: fixtureNow, Since: 24 * time.Hour}
	rows, err := st.StatsRead(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Outcomes) != 100000 {
		t.Fatalf("fixture holds %d outcomes, want 100000", len(rows.Outcomes))
	}
	db, err := sql.Open("sqlite", "file:"+rows.Path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, project := range []string{"", "acme/a"} {
		q.Project = project
		for _, s := range store.StatsStatements(q) {
			r, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+s.SQL, s.Args...)
			if err != nil {
				t.Fatalf("%s: %v", s.Name, err)
			}
			for r.Next() {
				var id, parent, unused int
				var detail string
				if err = r.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(detail, "SCAN ") && !strings.Contains(detail, "USING") && !strings.Contains(detail, "SUBQUERY") {
					t.Errorf("statement %s (project %q) scans a table: %s", s.Name, project, detail)
				}
			}
			_ = r.Close()
		}
	}
	budget := 500 * time.Millisecond
	if runtime.GOOS != "linux" {
		budget *= 10
	}
	if raceEnabled {
		budget *= 10 // the budget is a plain-build figure; the detector multiplies the cost of every memory access
	}
	// The budget bounds the work Compute does, not how busy the runner is: on a shared runner the same read runs
	// several times slower in wall time (120 ms idle, 870 ms with every package testing at once). CPU time of the
	// process is load-independent where the platform reports it; elsewhere cpuTime falls back to wall time.
	start := cpuTime()
	d, err := Compute(ctx, st, Options{Now: fixtureNow, Since: 24 * time.Hour, Global: true})
	if took := cpuTime() - start; err != nil || took > budget {
		t.Fatalf("Compute took %s of CPU (budget %s), err %v", took, budget, err)
	}
	if d.Attempts == nil || d.Attempts.Finished != 100000 {
		t.Fatalf("attempts = %+v", d.Attempts)
	}
}
