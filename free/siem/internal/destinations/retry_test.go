package destinations

import (
	"context"
	"errors"
	"testing"
)

type countShipper struct {
	calls int
	failN int // fail on first N calls
}

func (c *countShipper) Ship(_ context.Context, _ [][]byte) error {
	c.calls++
	if c.calls <= c.failN {
		return errors.New("simulated error")
	}
	return nil
}

func TestRetry_SucceedsOnSecondAttempt(t *testing.T) {
	inner := &countShipper{failN: 1}
	s := WithRetry(inner)

	err := s.Ship(context.Background(), [][]byte{[]byte(`{}`)})
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}
	if inner.calls != 2 {
		t.Errorf("expected 2 calls (1 fail + 1 success), got %d", inner.calls)
	}
}

func TestRetry_ExhaustsAfterThreeAttempts(t *testing.T) {
	inner := &countShipper{failN: 10} // always fails
	s := WithRetry(inner)

	err := s.Ship(context.Background(), [][]byte{[]byte(`{}`)})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if inner.calls != 3 {
		t.Errorf("expected 3 attempts, got %d", inner.calls)
	}
}

func TestRetry_SucceedsImmediately(t *testing.T) {
	inner := &countShipper{failN: 0}
	s := WithRetry(inner)

	err := s.Ship(context.Background(), [][]byte{[]byte(`{}`)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("expected 1 call, got %d", inner.calls)
	}
}
