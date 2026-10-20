package store

// Purpose: AttemptSourceTrust returns the trust class of the attempt's own pipeline and says "unknown" for
//          an attempt that does not exist, instead of a default.
// Inputs:  a seeded store. Outputs: pass or fail. Constraints: none.

import (
	"context"
	"testing"
)

func TestAttemptSourceTrust(t *testing.T) {
	s := testStore(t)
	seed(t, s, "a1")
	ctx := context.Background()
	for _, tc := range []struct {
		name, attempt, want string
		ok                  bool
	}{
		{"known attempt", "a1", "owner", true},
		{"unknown attempt", "nope", "", false},
		{"empty id", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := s.AttemptSourceTrust(ctx, tc.attempt)
			if err != nil || ok != tc.ok || got != tc.want {
				t.Fatalf("got (%q, %v, %v), want (%q, %v)", got, ok, err, tc.want, tc.ok)
			}
		})
	}
}
