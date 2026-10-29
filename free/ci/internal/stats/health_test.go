package stats

// Purpose: tests of the `health` doctor section (P7-SCHED-20, CI-STAT-01..05): the fixture's stale lease, two-hour
// outbox entry and day-old offline node warn; a machine with no run store passes every check; nothing fails.
// Inputs:  the fixture store; the section run with NSELF_CI_HOME pointing at it.
// Outputs: pass or fail.
// Constraints: the section reads the wall clock, so only conditions that age in one direction are asserted.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func statuses(checks []doctor.Check) map[string]doctor.Status {
	out := map[string]doctor.Status{}
	for _, c := range checks {
		out[c.ID] = c.Status
	}
	return out
}

func TestHealthSectionWarnsOnTheFixture(t *testing.T) {
	st := buildFixture(t, 40)
	rows, err := st.StatsRead(context.Background(), store.StatsQuery{Now: time.Now(), Since: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	env := doctor.Env{Getenv: func(k string) string {
		if k == "NSELF_CI_HOME" {
			return filepath.Dir(rows.Path)
		}
		return ""
	}}
	got := statuses(healthSection(context.Background(), env))
	for _, id := range []string{IDStaleLeases, IDOutbox, IDNodes} {
		if got[id] != doctor.Warn {
			t.Errorf("%s = %q, want warn (all: %v)", id, got[id], got)
		}
	}
	for _, id := range []string{IDStaleLeases, IDQueueWait, IDOutbox, IDNodes, IDStoreSize} {
		if got[id] == "" || got[id] == doctor.Fail {
			t.Errorf("%s = %q", id, got[id])
		}
	}
}

func TestHealthSectionWithoutAStorePasses(t *testing.T) {
	env := doctor.Env{Getenv: func(string) string { return t.TempDir() }}
	got := statuses(healthSection(context.Background(), env))
	if len(got) != 5 {
		t.Fatalf("checks = %v", got)
	}
	for id, s := range got {
		if s != doctor.Pass {
			t.Errorf("%s = %q, want pass", id, s)
		}
	}
}

func TestHealthThresholds(t *testing.T) {
	big, free := int64(300), int64(1000)
	wait, age := QueueWaitWarnMs+1, OutboxAgeWarnMs+1
	d := &Doc{Leases: &Leases{}, Queue: map[string]Band{"normal": {WaitMs: &Pct{P95: wait}}},
		Outbox: &Outbox{Pending: 3, OldestAgeMs: &age}, Nodes: &Nodes{}, Store: &StoreInfo{SizeBytes: big, FreeBytes: &free}}
	got := statuses(HealthChecks(d))
	for _, id := range []string{IDQueueWait, IDOutbox, IDStoreSize} {
		if got[id] != doctor.Warn {
			t.Errorf("%s = %q, want warn", id, got[id])
		}
	}
	if got[IDStaleLeases] != doctor.Pass || got[IDNodes] != doctor.Pass {
		t.Errorf("clean checks warned: %v", got)
	}
	d.Outbox = &Outbox{Pending: OutboxCountWarn + 1}
	if statuses(HealthChecks(d))[IDOutbox] != doctor.Warn {
		t.Error("more than 50 pending must warn")
	}
}
