package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestExpiredClaimSupersedingRequeuesNewest(t *testing.T) {
	for _, newestState := range []string{OutboxPosted, OutboxPosting} {
		t.Run(newestState, func(t *testing.T) {
			now := time.Now()
			s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{Clock: func() time.Time { return now }})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			ctx := context.Background()
			a, err := s.OutboxReserve(ctx, "rev", "nself-ci")
			if err != nil {
				t.Fatal(err)
			}
			b, err := s.OutboxReserve(ctx, "rev", "nself-ci")
			if err != nil {
				t.Fatal(err)
			}
			for _, seq := range []int64{a, b} {
				if err := s.OutboxReady(ctx, "rev", "nself-ci", seq, []byte(`{}`)); err != nil {
					t.Fatal(err)
				}
			}
			if result, _ := claim(s, a); result != ClaimOK {
				t.Fatal("older claim refused")
			}
			now = now.Add(claimTTL + time.Second)
			result, token := claim(s, b)
			if result != ClaimOK {
				t.Fatal("newer claim refused")
			}
			if newestState == OutboxPosted {
				if err := s.OutboxPosted(ctx, "rev", "nself-ci", b, token); err != nil {
					t.Fatal(err)
				}
			}
			if result, _ := claim(s, a); result != ClaimSuperseded {
				t.Fatalf("result %d", result)
			}
			pending, err := s.OutboxUnposted(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 1 || pending[0].Seq != b || pending[0].State != OutboxPending {
				t.Fatalf("newest status not requeued: %+v", pending)
			}
		})
	}
}
