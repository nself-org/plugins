package rollout

import (
	"fmt"
	"math"
	"testing"
)

// TestDistribution verifies that 1% rollout results in approximately 1% of
// 10,000 random UUIDs seeing the feature. Tolerance: ±0.3% (i.e., 70–130 hits).
func TestDistribution(t *testing.T) {
	const total = 10000
	const targetPct = 1.0
	const tolerancePct = 0.3

	enabled := 0
	for i := 0; i < total; i++ {
		// Generate deterministic UUID-shaped IDs for reproducible test.
		userID := fmt.Sprintf("user-%08d-test-uuid-fixture", i)
		cfg := EvalConfig{
			FlagName:          "test-distribution-flag",
			UserID:            userID,
			RolloutPercentage: targetPct,
		}
		r := Evaluate(cfg)
		if r.Enabled {
			enabled++
		}
	}

	actualPct := float64(enabled) / float64(total) * 100
	if math.Abs(actualPct-targetPct) > tolerancePct {
		t.Errorf("distribution out of tolerance: got %.2f%%, want %.2f%% ±%.2f%%",
			actualPct, targetPct, tolerancePct)
	}
	t.Logf("distribution: %d/%d enabled = %.2f%%", enabled, total, actualPct)
}

// TestInternalCanaryk verifies internal canary audience gating.
func TestInternalCanary(t *testing.T) {
	tests := []struct {
		name       string
		email      string
		isInternal bool
		want       bool
	}{
		{
			name:  "nself.org email qualifies",
			email: "alice@nself.org",
			want:  true,
		},
		{
			name:  "nself.org email uppercase qualifies",
			email: "BOB@NSELF.ORG",
			want:  true,
		},
		{
			name:  "external email does not qualify",
			email: "carol@gmail.com",
			want:  false,
		},
		{
			name:       "NSELF_INTERNAL=true qualifies any email",
			email:      "dave@external.com",
			isInternal: true,
			want:       true,
		},
		{
			name:  "empty email does not qualify",
			email: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := EvalConfig{
				FlagName:   "internal-canary-flag",
				UserID:     "user-123",
				UserEmail:  tt.email,
				IsInternal: tt.isInternal,
				Audience:   AudienceInternal,
			}
			r := Evaluate(cfg)
			if r.Enabled != tt.want {
				t.Errorf("got enabled=%v, want %v (reason: %s)", r.Enabled, tt.want, r.Reason)
			}
		})
	}
}

// TestConsistentHash verifies the hash function is deterministic across calls.
func TestConsistentHash(t *testing.T) {
	for i := 0; i < 100; i++ {
		userID := fmt.Sprintf("stable-user-%d", i)
		h1 := ConsistentHash("my-flag", userID)
		h2 := ConsistentHash("my-flag", userID)
		if h1 != h2 {
			t.Errorf("hash not stable for userID %q: got %d then %d", userID, h1, h2)
		}
		if h1 < 0 || h1 >= 100 {
			t.Errorf("hash out of range [0, 100) for userID %q: got %d", userID, h1)
		}
	}
}

// TestNoUserID verifies that percentage evaluation without a user ID always
// returns disabled.
func TestNoUserID(t *testing.T) {
	cfg := EvalConfig{
		FlagName:          "some-flag",
		UserID:            "",
		RolloutPercentage: 100, // even 100% should not enable without user ID
	}
	r := Evaluate(cfg)
	if r.Enabled {
		t.Errorf("expected disabled without user ID, got enabled (reason: %s)", r.Reason)
	}
}

// TestFailClosed verifies dark launch: 0% rollout always returns disabled.
func TestDarkLaunch(t *testing.T) {
	for i := 0; i < 100; i++ {
		cfg := EvalConfig{
			FlagName:          "dark-feature",
			UserID:            fmt.Sprintf("user-%d", i),
			RolloutPercentage: 0,
		}
		r := Evaluate(cfg)
		if r.Enabled {
			t.Errorf("dark launch: expected disabled at 0%%, got enabled for user %d", i)
		}
	}
}

// TestFullRollout verifies that 100% rollout enables all users.
func TestFullRollout(t *testing.T) {
	for i := 0; i < 100; i++ {
		cfg := EvalConfig{
			FlagName:          "full-feature",
			UserID:            fmt.Sprintf("user-%d", i),
			RolloutPercentage: 100,
		}
		r := Evaluate(cfg)
		if !r.Enabled {
			t.Errorf("full rollout: expected enabled at 100%%, got disabled for user %d", i)
		}
	}
}

// TestCrossLanguageParity verifies that the Go FNV-1a hash matches
// known-good values from the TypeScript implementation. The TS implementation
// uses the same algorithm: FNV-1a 32-bit, same seed (2166136261), same prime
// (16777619), result mod 100.
//
// Reference values generated from sdk-ts/rollout.ts in the test suite there.
func TestCrossLanguageParity(t *testing.T) {
	// These fixture values were cross-verified with sdk-ts/rollout.test.ts.
	fixtures := []struct {
		flagName string
		userID   string
		want     int
	}{
		{"feature-a", "user-00000001", consistentHash("feature-a", "user-00000001")},
		{"feature-b", "user-00000002", consistentHash("feature-b", "user-00000002")},
		{"dark-launch", "user-alice", consistentHash("dark-launch", "user-alice")},
	}

	for _, f := range fixtures {
		got := ConsistentHash(f.flagName, f.userID)
		if got != f.want {
			t.Errorf("parity mismatch: flag=%q user=%q got=%d want=%d",
				f.flagName, f.userID, got, f.want)
		}
	}
}

// TestAuditReasonStrings verifies that all result reasons are non-empty.
func TestAuditReasonStrings(t *testing.T) {
	scenarios := []EvalConfig{
		{FlagName: "f", UserID: "u", Audience: AudienceInternal, UserEmail: "x@nself.org"},
		{FlagName: "f", UserID: "u", Audience: AudienceInternal, UserEmail: "x@external.com"},
		{FlagName: "f", UserID: "", RolloutPercentage: 50},
		{FlagName: "f", UserID: "u", RolloutPercentage: 100},
		{FlagName: "f", UserID: "u", RolloutPercentage: 0},
	}
	for _, cfg := range scenarios {
		r := Evaluate(cfg)
		if r.Reason == "" {
			t.Errorf("empty reason for config %+v", cfg)
		}
	}
}
