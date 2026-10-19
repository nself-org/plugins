package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
)

func count(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.readers.QueryRowContext(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeletePipelinesRemovesTreeAndSkipsRunning(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	seed(t, s, "1")
	seed(t, s, "2")
	seed(t, s, "3")
	if err := s.FinishPipeline(ctx, "p1", "passed"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishPipeline(ctx, "p2", "failed"); err != nil {
		t.Fatal(err)
	}
	if err := s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE pipeline SET status='running' WHERE id='p3'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEvidence(ctx, "e1", "p1", "1", "rev", "i", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	n, err := s.DeletePipelines(ctx, []string{"p1", "p3", "p2", "nope"})
	if err != nil || n != 2 {
		t.Fatalf("removed %d, err %v; want 2", n, err)
	}
	ages, err := s.PipelineAges(ctx)
	if err != nil || len(ages) != 1 || ages[0].ID != "p3" {
		t.Fatalf("left %+v, err %v; want only the running p3", ages, err)
	}
	for table, want := range map[string]int{"job": 1, "attempt": 1, "evidence": 0} {
		if got := count(t, s, table); got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}
	}
	if got := count(t, s, "audit"); got < 1 {
		t.Error("the prune left no audit row")
	}
	if n, err = s.DeletePipelines(ctx, []string{"p3"}); err != nil || n != 0 {
		t.Errorf("a running pipeline was removed: n=%d err=%v", n, err)
	}
}

// A history longer than one batch goes in several transactions, and a running
// pipeline in the middle of a batch survives.
func TestDeletePipelinesInBatches(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	var ids []string
	for i := 0; i < pruneBatch+50; i++ {
		id := fmt.Sprintf("%03d", i)
		seed(t, s, id)
		ids = append(ids, "p"+id)
		if i != 7 {
			if err := s.FinishPipeline(ctx, "p"+id, "passed"); err != nil {
				t.Fatal(err)
			}
		} else if err := s.write(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE pipeline SET status='running' WHERE id='p007'`)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.DeletePipelines(ctx, ids)
	if err != nil || n != len(ids)-1 {
		t.Fatalf("removed %d of %d, err %v", n, len(ids), err)
	}
	if ages, _ := s.PipelineAges(ctx); len(ages) != 1 || ages[0].ID != "p007" {
		t.Fatalf("left %+v", ages)
	}
	if got := count(t, s, "attempt"); got != 1 {
		t.Fatalf("attempt rows left: %d", got)
	}
}
