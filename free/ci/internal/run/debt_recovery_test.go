package run

import (
	"context"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"path/filepath"
	"testing"
)

func TestSweepCoordinatorOwnership(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	}()
	live, err := st.RegisterCoordinator(ctx, 1, "live", "boot")
	if err != nil {
		t.Fatal(err)
	}
	dead, err := st.RegisterCoordinator(ctx, 2, "dead", "old-boot")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, id string
		want     bool
	}{
		{"live", safeID(live.ID), false}, {"stale", safeID(dead.ID), true}, {"other home", "unknown", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (staleChecker{st, "boot"}).Stale(ctx, tc.id)
			if err != nil || got != tc.want {
				t.Fatalf("Stale = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}
