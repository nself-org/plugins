// Package dunning implements the dunning engine: retry failed invoices on a
// schedule and cancel subscriptions after the grace period expires.
package dunning

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-payments/internal/config"
	"github.com/nself-org/nself-payments/internal/db"
	"github.com/nself-org/nself-payments/internal/metrics"
	"github.com/nself-org/nself-payments/internal/provider"
)

const checkInterval = 1 * time.Hour

// Engine runs the dunning cron loop.
type Engine struct {
	pool      *pgxpool.Pool
	providers map[string]provider.Provider
	cfg       *config.Config
	metrics   *metrics.Metrics
}

// New creates a new dunning Engine.
func New(
	pool *pgxpool.Pool,
	providers map[string]provider.Provider,
	cfg *config.Config,
	m *metrics.Metrics,
) *Engine {
	return &Engine{
		pool:      pool,
		providers: providers,
		cfg:       cfg,
		metrics:   m,
	}
}

// Start begins the dunning loop in a goroutine. The returned function stops it.
func (e *Engine) Start() func() {
	ctx, cancel := context.WithCancel(context.Background())
	go e.loop(ctx)
	return cancel
}

func (e *Engine) loop(ctx context.Context) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// Run once immediately at startup.
	e.run(ctx)

	for {
		select {
		case <-ticker.C:
			e.run(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) run(ctx context.Context) {
	slog.Info("dunning: starting cycle")

	if err := e.retryPastDue(ctx); err != nil {
		slog.Error("dunning: retry past due error", "error", err)
	}

	if err := e.cancelExpiredGrace(ctx); err != nil {
		slog.Error("dunning: cancel expired grace error", "error", err)
	}
}

// retryPastDue attempts invoice retry for past_due subscriptions following the
// configured retry schedule. Sets grace_period_end on first failure.
func (e *Engine) retryPastDue(ctx context.Context) error {
	subs, err := db.GetPastDueSubscriptions(ctx, e.pool)
	if err != nil {
		return fmt.Errorf("dunning: fetch past_due: %w", err)
	}

	for _, sub := range subs {
		prov, ok := e.providers[sub.Provider]
		if !ok {
			slog.Warn("dunning: provider not configured", "provider", sub.Provider, "sub_id", sub.ID)
			continue
		}

		// Determine which retry attempt we are on based on time since updated_at.
		attempt := e.retryAttempt(sub.UpdatedAt)
		if attempt < 0 {
			// Not yet due for the next retry attempt.
			continue
		}

		attemptStr := strconv.Itoa(attempt)
		e.metrics.DunningRetry.WithLabelValues(sub.Provider, attemptStr).Inc()

		result, err := prov.RetryInvoice(ctx, sub.ProviderSubID)
		if err != nil {
			slog.Error("dunning: retry invoice failed", "provider", sub.Provider, "sub_id", sub.ID, "error", err)
			continue
		}

		if result.Success {
			slog.Info("dunning: retry succeeded", "provider", sub.Provider, "sub_id", sub.ID, "attempt", attempt)
			e.metrics.DunningTransitions.WithLabelValues("past_due", "active").Inc()
		} else {
			slog.Info("dunning: retry failed", "provider", sub.Provider, "sub_id", sub.ID, "attempt", attempt, "reason", result.Error)

			// Set grace_period_end if not already set.
			if sub.GracePeriodEnd == nil {
				graceDays := time.Duration(e.cfg.DunningGraceDays) * 24 * time.Hour
				graceEnd := time.Now().Add(graceDays)
				if err := db.SetGracePeriodEnd(ctx, e.pool, sub.ID, graceEnd); err != nil {
					slog.Error("dunning: set grace period", "sub_id", sub.ID, "error", err)
				}
			}
		}
	}
	return nil
}

// cancelExpiredGrace cancels subscriptions whose grace period has expired.
func (e *Engine) cancelExpiredGrace(ctx context.Context) error {
	subs, err := db.GetExpiredGracePeriods(ctx, e.pool)
	if err != nil {
		return fmt.Errorf("dunning: fetch expired grace: %w", err)
	}

	for _, sub := range subs {
		prov, ok := e.providers[sub.Provider]
		if !ok {
			continue
		}

		if err := prov.CancelSubscription(ctx, sub.ProviderSubID); err != nil {
			slog.Error("dunning: cancel subscription failed", "provider", sub.Provider, "sub_id", sub.ID, "error", err)
			continue
		}

		slog.Info("dunning: canceled after grace expiry", "provider", sub.Provider, "sub_id", sub.ID)
		e.metrics.DunningTransitions.WithLabelValues("past_due", "canceled").Inc()
	}
	return nil
}

// retryAttempt returns which retry attempt (0-indexed) is due based on elapsed
// time since the subscription entered past_due state. Returns -1 if no retry
// is due yet.
func (e *Engine) retryAttempt(since time.Time) int {
	elapsed := time.Since(since)
	for i, days := range e.cfg.DunningRetryIntervals {
		threshold := time.Duration(days) * 24 * time.Hour
		if elapsed >= threshold {
			// Still within the next interval's window (or last interval).
			if i+1 < len(e.cfg.DunningRetryIntervals) {
				nextThreshold := time.Duration(e.cfg.DunningRetryIntervals[i+1]) * 24 * time.Hour
				if elapsed < nextThreshold {
					return i
				}
			} else {
				return i
			}
		}
	}
	return -1
}
