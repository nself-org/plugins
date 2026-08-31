package ratelimit_test

import (
	"context"
	"testing"

	"github.com/nself-org/plugins-pro/paid/nself-cloud/internal/ratelimit"
)

func TestMemoryLimiter_Allow(t *testing.T) {
	// t.Setenv requires no t.Parallel on same test.
	t.Setenv("REDIS_URL", "")
	limiter := ratelimit.New()
	ctx := context.Background()

	const tenantID = "tenant-abc-123"

	// First 5 calls should succeed.
	for i := 0; i < 5; i++ {
		if err := limiter.Allow(ctx, tenantID); err != nil {
			t.Fatalf("call %d: expected nil, got %v", i+1, err)
		}
	}

	// 6th call should be rejected.
	if err := limiter.Allow(ctx, tenantID); err == nil {
		t.Fatal("6th call: expected ErrRateLimitExceeded, got nil")
	}
}

func TestMemoryLimiter_DifferentTenants(t *testing.T) {
	t.Setenv("REDIS_URL", "")
	limiter := ratelimit.New()
	ctx := context.Background()

	// Exhaust tenant-A.
	for i := 0; i < 5; i++ {
		if err := limiter.Allow(ctx, "tenant-A"); err != nil {
			t.Fatalf("tenant-A call %d: %v", i+1, err)
		}
	}
	if err := limiter.Allow(ctx, "tenant-A"); err == nil {
		t.Fatal("tenant-A 6th: expected error")
	}

	// tenant-B should still have a fresh window.
	if err := limiter.Allow(ctx, "tenant-B"); err != nil {
		t.Fatalf("tenant-B should not be rate-limited: %v", err)
	}
}
