// Package mfa — throttle.go
//
// In-process brute-force throttle for MFA verification attempts.
// Mirrors the MFAThrottle in the base auth plugin but lives here to avoid
// an import cycle between auth and auth-enterprise.
package mfa

import (
	"sync"
	"time"
)

// Throttle tracks failed MFA verification attempts per key (typically user_id).
type Throttle struct {
	mu          sync.Mutex
	maxFailures int
	window      time.Duration
	cooldown    time.Duration
	state       map[string]*throttleEntry
}

type throttleEntry struct {
	failures  int
	firstFail time.Time
	lockedAt  time.Time
}

// NewThrottle constructs a Throttle.
// maxFailures failures within window locks the key for cooldown duration.
func NewThrottle(maxFailures int, window, cooldown time.Duration) *Throttle {
	return &Throttle{
		maxFailures: maxFailures,
		window:      window,
		cooldown:    cooldown,
		state:       make(map[string]*throttleEntry),
	}
}

// IsLocked reports whether key is currently in a cooldown window.
func (t *Throttle) IsLocked(key string) bool {
	if t == nil || key == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.state[key]
	if !ok || e.lockedAt.IsZero() {
		return false
	}
	if time.Since(e.lockedAt) >= t.cooldown {
		delete(t.state, key)
		return false
	}
	return true
}

// RecordFailure increments the failure counter for key.
func (t *Throttle) RecordFailure(key string) {
	if t == nil || key == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	e, ok := t.state[key]
	if !ok {
		t.state[key] = &throttleEntry{failures: 1, firstFail: now}
		return
	}
	if now.Sub(e.firstFail) > t.window {
		e.failures = 1
		e.firstFail = now
		e.lockedAt = time.Time{}
		return
	}
	e.failures++
	if e.failures >= t.maxFailures {
		e.lockedAt = now
	}
}

// Reset clears all throttle state for key (call after a successful verification).
func (t *Throttle) Reset(key string) {
	if t == nil || key == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.state, key)
}
