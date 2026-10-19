package store

// D-1008: EvidenceForPipeline reads one run's own documents. The revision lookup
// returned every document of every run that shared a revision and input digest,
// so a history scan read each body once per run (quadratic in the history).

import (
	"context"
	"testing"
)

func TestEvidenceForPipelineReturnsOnlyThatRunsDocument(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seed(t, s, "a") // pipelines pa and pb share revision "rev" and input digest "i"
	seed(t, s, "b")
	for _, run := range []string{"a", "b"} {
		if err := s.RecordRunEvidence(ctx, "ev-"+run, "p"+run, "rev", "i", []byte("run "+run)); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordEvidence(ctx, "att-"+run, "p"+run, run, "rev", "i", []byte("attempt "+run)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.EvidenceForPipeline(ctx, "pa", "rev", "i")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "ev-a" || string(got[0].Body) != "run a" || got[0].AttemptID != "" {
		t.Fatalf("pipeline pa: %+v", got)
	}
	if none, err := s.EvidenceForPipeline(ctx, "pa", "rev", "other"); err != nil || len(none) != 0 {
		t.Fatalf("another input digest must match nothing: %+v %v", none, err)
	}
}
