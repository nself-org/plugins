package dunning

import (
	"testing"
	"time"

	"github.com/nself-org/nself-payments/internal/config"
	"github.com/nself-org/nself-payments/internal/metrics"
)

// sharedMetrics is created once per test binary to avoid duplicate Prometheus registration.
var sharedMetrics = metrics.New()

func newTestEngine(intervals []int, graceDays int) *Engine {
	return &Engine{
		cfg: &config.Config{
			DunningRetryIntervals: intervals,
			DunningGraceDays:      graceDays,
		},
		metrics: sharedMetrics,
	}
}

// TestRetryAttempt_BeforeFirstInterval returns -1 when not yet due.
func TestRetryAttempt_BeforeFirstInterval(t *testing.T) {
	e := newTestEngine([]int{1, 3, 7}, 7)
	// 12 hours elapsed — not past 1 day.
	since := time.Now().Add(-12 * time.Hour)
	got := e.retryAttempt(since)
	if got != -1 {
		t.Errorf("expected -1 (not due), got %d", got)
	}
}

// TestRetryAttempt_AfterFirstInterval returns 0 at 1-day mark.
func TestRetryAttempt_AfterFirstInterval(t *testing.T) {
	e := newTestEngine([]int{1, 3, 7}, 7)
	// 25 hours elapsed — past 1 day but before 3 days.
	since := time.Now().Add(-25 * time.Hour)
	got := e.retryAttempt(since)
	if got != 0 {
		t.Errorf("expected 0 (first retry), got %d", got)
	}
}

// TestRetryAttempt_AfterSecondInterval returns 1 at 3-day mark.
func TestRetryAttempt_AfterSecondInterval(t *testing.T) {
	e := newTestEngine([]int{1, 3, 7}, 7)
	// 4 days elapsed — past 3 days but before 7 days.
	since := time.Now().Add(-4 * 24 * time.Hour)
	got := e.retryAttempt(since)
	if got != 1 {
		t.Errorf("expected 1 (second retry), got %d", got)
	}
}

// TestRetryAttempt_AfterLastInterval returns last index when past final threshold.
func TestRetryAttempt_AfterLastInterval(t *testing.T) {
	e := newTestEngine([]int{1, 3, 7}, 7)
	// 8 days elapsed — past the final 7-day threshold.
	since := time.Now().Add(-8 * 24 * time.Hour)
	got := e.retryAttempt(since)
	if got != 2 {
		t.Errorf("expected 2 (last retry), got %d", got)
	}
}

// TestRetryAttempt_SingleInterval works with a one-item interval list.
func TestRetryAttempt_SingleInterval(t *testing.T) {
	e := newTestEngine([]int{3}, 7)
	// 4 days elapsed — past 3 days.
	since := time.Now().Add(-4 * 24 * time.Hour)
	got := e.retryAttempt(since)
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// TestRetryAttempt_SingleInterval_NotDue is before the threshold.
func TestRetryAttempt_SingleInterval_NotDue(t *testing.T) {
	e := newTestEngine([]int{3}, 7)
	since := time.Now().Add(-1 * 24 * time.Hour)
	got := e.retryAttempt(since)
	if got != -1 {
		t.Errorf("expected -1, got %d", got)
	}
}

// TestRetryAttempt_EmptyIntervals returns -1 with empty list.
func TestRetryAttempt_EmptyIntervals(t *testing.T) {
	e := newTestEngine([]int{}, 7)
	since := time.Now().Add(-8 * 24 * time.Hour)
	got := e.retryAttempt(since)
	if got != -1 {
		t.Errorf("expected -1 (empty intervals), got %d", got)
	}
}

// TestGracePeriodDuration verifies grace days config produces correct duration.
func TestGracePeriodDuration(t *testing.T) {
	e := newTestEngine([]int{1, 3, 7}, 7)
	graceDuration := time.Duration(e.cfg.DunningGraceDays) * 24 * time.Hour
	expected := 7 * 24 * time.Hour
	if graceDuration != expected {
		t.Errorf("expected %v grace duration, got %v", expected, graceDuration)
	}
}
