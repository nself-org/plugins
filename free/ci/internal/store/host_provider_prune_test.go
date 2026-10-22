package store

import (
	"context"
	"testing"
	"time"
)

func TestProviderMarkRetention(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	now := time.Unix(1700000000, 0)
	for _, tc := range []struct {
		name, key string
		age       time.Duration
		kept      bool
	}{
		{"github-reconcile", "term|old", 31 * 24 * time.Hour, false},
		{"github-reconcile", "term|recent", 8 * 24 * time.Hour, true},
		{"github-reconcile", "run|old", 8 * 24 * time.Hour, false},
		{"github-reconcile", "demand|old", 8 * 24 * time.Hour, false},
		{"github-reconcile", "run|boundary", 7 * 24 * time.Hour, true},
		{"other", "run|old", 40 * 24 * time.Hour, true},
	} {
		t.Run(tc.key+tc.name, func(t *testing.T) {
			if err := s.ProviderMarkPut(ctx, tc.name, tc.key, now.Add(-tc.age)); err != nil {
				t.Fatal(err)
			}
			if err := s.PruneReconcileMarks(ctx, now); err != nil {
				t.Fatal(err)
			}
			if _, ok, err := s.ProviderMarkGet(ctx, tc.name, tc.key); err != nil || ok != tc.kept {
				t.Fatalf("kept=%v want=%v err=%v", ok, tc.kept, err)
			}
		})
	}
}
