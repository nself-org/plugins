package exec

import (
	"context"
	"testing"
)

// An executor without a stale checker sweeps nothing and reports no error.
func TestSweepStaleProcessesWithoutChecker(t *testing.T) {
	if err := (&Executor{}).SweepStaleProcesses(context.Background(), t.TempDir(), "ag-live"); err != nil {
		t.Fatal(err)
	}
}

// A home with no process records is a clean sweep.
func TestSweepStaleProcessesEmptyHome(t *testing.T) {
	ex := &Executor{Stale: staleNever{}}
	if err := ex.SweepStaleProcesses(context.Background(), t.TempDir(), "ag-live"); err != nil {
		t.Fatal(err)
	}
}

type staleNever struct{}

func (staleNever) Stale(context.Context, string) (bool, error) { return false, nil }
