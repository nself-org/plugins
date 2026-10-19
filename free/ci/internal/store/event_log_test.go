package store

import (
	"context"
	"sync"
	"testing"
)

func TestEventLogAppendsInOrderAndReadsFromASeq(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	seed(t, s, "1")
	for i, typ := range []string{"queued", "planned", "started"} {
		seq, err := s.AppendEvent(ctx, "p1", typ, `{"job":"a"}`)
		if err != nil || seq != int64(i+1) {
			t.Fatalf("append %s: seq %d err %v", typ, seq, err)
		}
	}
	if seq, err := s.AppendEvent(ctx, "other", "queued", "{}"); err != nil || seq != 1 {
		t.Fatalf("a second run starts at seq 1: %d %v", seq, err)
	}
	rows, err := s.EventRows(ctx, "p1", 2)
	if err != nil || len(rows) != 2 || rows[0].Seq != 2 || rows[0].Type != "planned" || rows[1].Seq != 3 || rows[1].Ref != `{"job":"a"}` {
		t.Fatalf("rows from 2: %+v %v", rows, err)
	}
	if rows, err = s.EventRows(ctx, "p1", 0); err != nil || len(rows) != 3 {
		t.Fatalf("rows from 0: %+v %v", rows, err)
	}
}

func TestEventLogSeqsStayUniqueUnderConcurrentAppends(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	seed(t, s, "1")
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.AppendEvent(ctx, "p1", "log", "{}"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	rows, err := s.EventRows(ctx, "p1", 0)
	if err != nil || len(rows) != 20 {
		t.Fatalf("%d rows, %v", len(rows), err)
	}
	for i, r := range rows {
		if r.Seq != int64(i+1) {
			t.Fatalf("row %d has seq %d", i, r.Seq)
		}
	}
}

func TestEventLogGoesWithItsPipeline(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	seed(t, s, "1")
	if _, err := s.AppendEvent(ctx, "p1", "queued", "{}"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishPipeline(ctx, "p1", "passed"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeletePipelines(ctx, []string{"p1"}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "event_index"); n != 0 {
		t.Fatalf("%d event rows survive the pipeline", n)
	}
}
