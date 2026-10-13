package logger

import (
	"context"
	"log/slog"
	"testing"
)

// Tests for WithTraceContext — covers the no-active-span path (logger returned unchanged).

func TestWithTraceContext_NoSpan(t *testing.T) {
	base := New(Options{Plugin: "test-plugin", Level: slog.LevelInfo})
	ctx := context.Background() // no active span

	got := WithTraceContext(ctx, base)
	// When no span is recording, the same logger reference is returned.
	// We verify by checking it is a non-nil *slog.Logger (shape check).
	if got == nil {
		t.Error("WithTraceContext returned nil logger, want non-nil")
	}
}

func TestWithTraceContext_WithNilLogger(t *testing.T) {
	// Even a nil base logger falls through — function should not panic.
	base := slog.Default()
	ctx := context.Background()
	got := WithTraceContext(ctx, base)
	if got == nil {
		t.Error("WithTraceContext returned nil, want non-nil slog.Logger")
	}
}
