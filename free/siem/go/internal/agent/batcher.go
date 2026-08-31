// batcher.go — periodic batch collection and flush trigger.
package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nself-org/nself-siem/internal/config"
)

// ShipFunc is called with a slice of normalized JSON payloads ready to ship.
type ShipFunc func(ctx context.Context, events [][]byte) error

// Batcher reads from np_audit_log on a configurable interval and calls ship.
// It maintains a cursor (last seen created_at) in memory and writes it to
// np_siem_destinations.last_shipped_at after each successful flush.
type Batcher struct {
	cfg    *config.Config
	pool   *pgxpool.Pool
	ship   ShipFunc
	schema string
	cursor time.Time
}

// NewBatcher creates a Batcher that flushes every cfg.FlushIntervalS seconds.
func NewBatcher(cfg *config.Config, pool *pgxpool.Pool, schema string, ship ShipFunc) *Batcher {
	return &Batcher{
		cfg:    cfg,
		pool:   pool,
		ship:   ship,
		schema: schema,
		cursor: time.Now().Add(-time.Minute), // start 1 min back to catch recent events
	}
}

// Run starts the batcher loop; blocks until ctx is cancelled.
func (b *Batcher) Run(ctx context.Context) {
	flushInterval := time.Duration(b.cfg.FlushIntervalS) * time.Second
	// High-frequency mode: minimum 5 s floor unless Enterprise flag set.
	if b.cfg.HighFreq && flushInterval < 5*time.Second {
		flushInterval = 5 * time.Second
	} else if !b.cfg.HighFreq && flushInterval < 5*time.Second {
		flushInterval = 30 * time.Second
	}

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := b.flush(ctx); err != nil {
				slog.Error("batcher flush error", "err", err)
			}
		}
	}
}

// flush reads a batch and ships it.
func (b *Batcher) flush(ctx context.Context) error {
	rows, err := ReadAuditRows(ctx, b.pool, b.cursor, b.cfg.BatchSize)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	events := make([][]byte, 0, len(rows))
	for _, row := range rows {
		data, err := NormalizeEvent(row, b.schema, "1.0.0")
		if err != nil {
			slog.Warn("normalizer error", "row", row.ID, "err", err)
			continue
		}
		events = append(events, data)
	}

	if err := b.ship(ctx, events); err != nil {
		return err
	}

	// Advance cursor to last row's timestamp.
	b.cursor = rows[len(rows)-1].CreatedAt
	slog.Info("batch shipped", "count", len(events), "cursor", b.cursor)
	return nil
}
