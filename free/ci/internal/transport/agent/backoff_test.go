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
