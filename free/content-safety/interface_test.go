package contentsafety

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockScanner is a test implementation that demonstrates fail-closed semantics.
type mockScanner struct {
	err error
	res ScanResult
}

func (m *mockScanner) Scan(ctx context.Context, content []byte) (ScanResult, error) {
	if err := ctx.Err(); err != nil {
		return ScanResult{}, ErrScanFailed
	}
	if m.err != nil {
		return ScanResult{}, m.err
	}
	return m.res, nil
}

func TestScanner_FailClosed_OnTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire.
	<-ctx.Done()

	scanner := &mockScanner{res: ScanResult{Safe: true}}
	_, err := scanner.Scan(ctx, []byte("test"))
	if !errors.Is(err, ErrScanFailed) {
		t.Errorf("expected ErrScanFailed on timeout, got %v", err)
	}
}

func TestScanner_FailClosed_OnError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	scanner := &mockScanner{err: errors.Join(errors.New("backend down"), ErrScanFailed)}
	_, err := scanner.Scan(ctx, []byte("test"))
	if !errors.Is(err, ErrScanFailed) {
		t.Errorf("expected ErrScanFailed on error, got %v", err)
	}
}
