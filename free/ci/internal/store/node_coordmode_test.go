package store

// Purpose: tests of the coordinator mode row (P7-NODE-20, Epic D21).
// Inputs:  temporary stores.
// Outputs: pass or fail.
// Constraints: no network; audit rows are counted through AuditEntries.

import (
	"context"
	"testing"
)

func modeAuditRows(t *testing.T, s *Store) int {
	t.Helper()
	rows, err := s.AuditEntries(context.Background(), 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range rows {
		if r.Action == "coordinator.mode" {
			n++
		}
	}
	return n
}

func TestCoordinatorModeStore(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		steps []bool // the --team flag of each start
		mode  string
		audit int
	}{
		{"fresh start is personal", []bool{false}, CoordinatorPersonal, 0},
		{"personal start repeated", []bool{false, false}, CoordinatorPersonal, 0},
		{"team switches once", []bool{false, true}, CoordinatorTeam, 1},
		{"team on a fresh dir", []bool{true}, CoordinatorTeam, 1},
		{"team stays team without the flag", []bool{false, true, false, false}, CoordinatorTeam, 1},
		{"team flag repeated writes no second row", []bool{true, true}, CoordinatorTeam, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			var got CoordinatorModeChange
			for _, team := range tc.steps {
				var err error
				if got, err = s.CoordinatorModeEnsure(ctx, team, "operator:serve"); err != nil {
					t.Fatal(err)
				}
			}
			if got.Mode != tc.mode || s.CoordinatorMode(ctx) != tc.mode {
				t.Fatalf("mode = %q / %q, want %q", got.Mode, s.CoordinatorMode(ctx), tc.mode)
			}
			if n := modeAuditRows(t, s); n != tc.audit {
				t.Fatalf("audit rows = %d, want %d", n, tc.audit)
			}
		})
	}
}

func TestCoordinatorModeFailClosed(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	if m, found, err := s.CoordinatorModeGet(ctx); m != "" || found || err != nil {
		t.Fatalf("no row: %q %v %v", m, found, err)
	}
	if s.CoordinatorMode(ctx) != CoordinatorTeam {
		t.Fatal("a missing row must report team")
	}
	if _, err := s.CoordinatorModeEnsure(ctx, false, "operator:serve"); err != nil {
		t.Fatal(err)
	}
	if s.CoordinatorMode(ctx) != CoordinatorPersonal {
		t.Fatal("personal row must report personal")
	}
	_ = s.Close()
	if m, _, err := s.CoordinatorModeGet(ctx); err == nil || m != CoordinatorTeam {
		t.Fatalf("an unreadable row must report team with the error: %q %v", m, err)
	}
	if s.CoordinatorMode(ctx) != CoordinatorTeam {
		t.Fatal("an unreadable row must report team")
	}
	if ch, err := s.CoordinatorModeEnsure(ctx, false, "operator:serve"); err == nil || ch.Mode != CoordinatorTeam {
		t.Fatalf("ensure on a closed store: %+v %v", ch, err)
	}
}

func TestCoordinatorModeNeverBackToPersonal(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	if _, err := s.CoordinatorModeEnsure(ctx, true, "operator:serve"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE coordinator_mode SET mode='personal' WHERE id=1`,
		`DELETE FROM coordinator_mode WHERE id=1`,
	} {
		if _, err := s.writer.ExecContext(ctx, q); err == nil {
			t.Fatalf("%q was accepted on a team row", q)
		}
	}
	if s.CoordinatorMode(ctx) != CoordinatorTeam {
		t.Fatal("the row changed")
	}
}
