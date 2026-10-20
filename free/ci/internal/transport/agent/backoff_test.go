package agent

// Purpose: tests of the reconnect backoff: the doubling ceiling, the 60 s cap, full jitter and the floor.
// Inputs:  a fixed random source. Constraints: exact bounds, no sleeping.

import (
	"testing"
	"time"
)

func fixed(v float64) func() float64 { return func() float64 { return v } }

func TestBackoffDoublesFromOneSecondToSixty(t *testing.T) {
	b := &Backoff{Rand: fixed(0.999999)}
	var got []time.Duration
	for i := 0; i < 9; i++ {
		got = append(got, b.Next().Round(time.Millisecond))
	}
	want := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60, 60}
	for i, w := range want {
		if d := w*time.Second - got[i]; d < 0 || d > 5*time.Millisecond {
			t.Fatalf("attempt %d waited %v, want just under %v", i, got[i], w*time.Second)
		}
	}
}

func TestBackoffIsFullJitter(t *testing.T) {
	for _, c := range []struct {
		r    float64
		want time.Duration
	}{{0, backoffFloor}, {0.5, 4 * time.Second}, {0.25, 2 * time.Second}} {
		b := &Backoff{Rand: fixed(c.r)}
		b.attempt = 3 // ceiling 8 s
		if got := b.Next(); got != c.want {
			t.Fatalf("r=%v waited %v, want %v", c.r, got, c.want)
		}
	}
}

func TestBackoffResetAndCustomBounds(t *testing.T) {
	b := &Backoff{Min: 10 * time.Millisecond, Max: 40 * time.Millisecond, Rand: fixed(0.999999)}
	for i := 0; i < 5; i++ {
		if d := b.Next(); d > 40*time.Millisecond {
			t.Fatalf("attempt %d waited %v above the max", i, d)
		}
	}
	b.Reset()
	if d := b.Next(); d > 10*time.Millisecond || d < time.Millisecond {
		t.Fatalf("after reset waited %v", d)
	}
}

func TestBackoffDefaultRandomStaysInRange(t *testing.T) {
	b := &Backoff{}
	for i := 0; i < 50; i++ {
		if d := b.Next(); d < backoffFloor || d > BackoffMax {
			t.Fatalf("delay %v out of range", d)
		}
	}
}

func TestBackoffMinEqualToMaxNeverGrows(t *testing.T) {
	b := &Backoff{Min: 2 * time.Second, Max: 2 * time.Second, Rand: fixed(0.999999)}
	for i := 0; i < 6; i++ {
		if d := b.Next(); d > 2*time.Second || d < 1999*time.Millisecond {
			t.Fatalf("attempt %d waited %v, want just under 2 s every time", i, d)
		}
	}
}

func TestBackoffMaxBelowMinFallsBackToTheDefaultMax(t *testing.T) {
	b := &Backoff{Min: 10 * time.Second, Max: 5 * time.Second, Rand: fixed(0.999999)}
	want := []time.Duration{10, 20, 40, 60, 60}
	for i, w := range want {
		if d := b.Next(); w*time.Second-d < 0 || w*time.Second-d > 5*time.Millisecond {
			t.Fatalf("attempt %d waited %v, want just under %v", i, d, w*time.Second)
		}
	}
}

func TestBackoffNonPositiveMinUsesTheDefaultAndTheCounterIsBounded(t *testing.T) {
	b := &Backoff{Min: -time.Second, Max: 0, Rand: fixed(0.999999)}
	if d := b.Next(); d > time.Second || d < 999*time.Millisecond {
		t.Fatalf("first delay %v, want just under the default 1 s", d)
	}
	for i := 0; i < 1000; i++ {
		b.Next()
	}
	if b.attempt != maxAttempts {
		t.Fatalf("attempt counter %d, want it held at %d", b.attempt, maxAttempts)
	}
	if d := b.Next(); d > BackoffMax {
		t.Fatalf("delay %v above the max", d)
	}
}

func TestBackoffMinOfZeroMeansOneSecondNotNoDelay(t *testing.T) {
	b := &Backoff{Min: 0, Max: 4 * time.Second, Rand: fixed(0.5)}
	if d := b.Next(); d != 500*time.Millisecond {
		t.Fatalf("delay %v, want half of the default 1 s", d)
	}
}

// The reconnect delay never exceeds a tenth of the agent_ttl the coordinator announced (floor: the minimum
// delay), so an agent with a job is back before it fences; before any welcome the delay is untouched.
func TestCapBackoffByAgentTTL(t *testing.T) {
	for _, c := range []struct {
		name         string
		d, ttl, want time.Duration
	}{
		{"no welcome yet", 50 * time.Second, 0, 50 * time.Second},
		{"default agent_ttl 25 s", 50 * time.Second, 25 * time.Second, 2500 * time.Millisecond},
		{"already short", 2 * time.Second, 25 * time.Second, 2 * time.Second},
		{"tiny agent_ttl keeps the floor", 50 * time.Second, 2 * time.Second, BackoffMin},
	} {
		if got := capBackoff(c.d, c.ttl); got != c.want {
			t.Errorf("%s: capBackoff(%v, %v) = %v, want %v", c.name, c.d, c.ttl, got, c.want)
		}
	}
}
