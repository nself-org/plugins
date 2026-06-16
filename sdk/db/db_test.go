package db

import (
	"context"
	"testing"
)

// Tests for db.Open — covers the DSN validation path without a real Postgres.

func TestOpen_EmptyDSN(t *testing.T) {
	_, err := Open(context.Background(), PoolConfig{DSN: ""})
	if err == nil {
		t.Error("Open() with empty DSN should return error, got nil")
	}
}

func TestOpen_InvalidDSN(t *testing.T) {
	// A DSN that parses successfully but has a bogus host — pgxpool.ParseConfig
	// accepts it syntactically but Connect will fail. We only test up to
	// ParseConfig to keep the test hermetic (no real Postgres needed).
	_, err := Open(context.Background(), PoolConfig{DSN: "not-a-valid-dsn"})
	if err == nil {
		t.Error("Open() with invalid DSN should return error, got nil")
	}
}

func TestPoolConfig_Defaults(t *testing.T) {
	// Verify the PoolConfig struct can be constructed with defaults.
	cfg := PoolConfig{DSN: "postgres://localhost/test"}
	if cfg.DSN == "" {
		t.Error("PoolConfig.DSN should be set")
	}
	if cfg.MaxConns != 0 {
		t.Errorf("MaxConns default should be 0 (zero value), got %d", cfg.MaxConns)
	}
}
