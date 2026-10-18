package destinations

import (
	"context"
	"log/slog"
	"time"
)

// Shipper is any destination that can receive a batch of events.
type Shipper interface {
	Ship(ctx context.Context, events [][]byte) error
}

// WithRetry wraps a Shipper with 3-attempt exponential backoff.
// Backoff: 1s, 2s, 4s. Context cancellation stops retry immediately.
func WithRetry(s Shipper) Shipper {
	return &retryShipper{inner: s}
}

type retryShipper struct {
	inner Shipper
}

func (r *retryShipper) Ship(ctx context.Context, events [][]byte) error {
	var lastErr error
	backoff := time.Second
	for attempt := 0; attempt < 3; attempt++ {
		lastErr = r.inner.Ship(ctx, events)
		if lastErr == nil {
			return nil
		}
		slog.Warn("ship attempt failed, retrying", "attempt", attempt+1, "backoff", backoff, "err", lastErr)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return lastErr
}
