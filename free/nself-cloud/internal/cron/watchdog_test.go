package cron

import (
	"context"
	"testing"
)

// mockAlerter records calls for test assertions.
type mockAlerter struct {
	alerts []string
}

func (m *mockAlerter) Alert(_ context.Context, instanceID, reason string) error {
	m.alerts = append(m.alerts, instanceID+":"+reason)
	return nil
}

func TestNewWatchdog_DefaultAlerter(t *testing.T) {
	w := NewWatchdog(nil, nil)
	if w == nil {
		t.Fatal("NewWatchdog returned nil")
	}
	if w.alerter == nil {
		t.Fatal("default alerter must not be nil")
	}
}

func TestNewWatchdog_CustomAlerter(t *testing.T) {
	a := &mockAlerter{}
	w := NewWatchdog(nil, a)
	if w.alerter != a {
		t.Fatal("custom alerter not set")
	}
}

// TestLogAlerter_Alert ensures the no-op alerter doesn't panic or error.
func TestLogAlerter_Alert(t *testing.T) {
	la := &logAlerter{}
	err := la.Alert(context.Background(), "inst-test", "some reason")
	if err != nil {
		t.Errorf("logAlerter.Alert should always return nil, got: %v", err)
	}
}

// TestWatchdogInterval_Constants ensures constants are sane.
func TestWatchdogConstants(t *testing.T) {
	if watchdogInterval <= 0 {
		t.Error("watchdogInterval must be positive")
	}
	if stuckThreshold <= watchdogInterval {
		t.Errorf("stuckThreshold (%s) should be > watchdogInterval (%s)", stuckThreshold, watchdogInterval)
	}
}
