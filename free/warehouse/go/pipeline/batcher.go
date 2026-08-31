package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nself-org/nself-warehouse/driver"
)

// Batcher accumulates CDC events and flushes them to the warehouse in
// size- or time-bounded batches.
type Batcher struct {
	cfg    *Config
	drv    driver.Driver
	store  *Store // postgres watermark + error store
	mu     sync.Mutex
	queues map[string][]CDCEvent // keyed by table name
}

func newBatcher(cfg *Config, drv driver.Driver, store *Store) *Batcher {
	return &Batcher{
		cfg:    cfg,
		drv:    drv,
		store:  store,
		queues: make(map[string][]CDCEvent),
	}
}

// Add appends an event to the in-memory queue for its table.
// If the queue reaches BatchSize it is flushed immediately.
func (b *Batcher) Add(ctx context.Context, evt CDCEvent) error {
	b.mu.Lock()
	b.queues[evt.Table] = append(b.queues[evt.Table], evt)
	ready := len(b.queues[evt.Table]) >= b.cfg.BatchSize
	b.mu.Unlock()

	if ready {
		return b.FlushTable(ctx, evt.Table)
	}
	return nil
}

// RunFlushLoop flushes all pending queues on the configured interval.
// This ensures rows are exported even when volume is low.
func (b *Batcher) RunFlushLoop(ctx context.Context) {
	ticker := time.NewTicker(b.cfg.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.mu.Lock()
			tables := make([]string, 0, len(b.queues))
			for t := range b.queues {
				tables = append(tables, t)
			}
			b.mu.Unlock()

			for _, t := range tables {
				if err := b.FlushTable(ctx, t); err != nil {
					slog.Warn("flush error", "table", t, "err", err)
				}
			}
		}
	}
}

// FlushTable drains the queue for one table and writes to the warehouse.
func (b *Batcher) FlushTable(ctx context.Context, table string) error {
	b.mu.Lock()
	events := b.queues[table]
	if len(events) == 0 {
		b.mu.Unlock()
		return nil
	}
	b.queues[table] = nil
	b.mu.Unlock()

	batchID := fmt.Sprintf("%s-%d", table, time.Now().UnixNano())
	rows := make([]map[string]interface{}, 0, len(events))
	var lastLSN string
	for _, e := range events {
		if e.Op != "DELETE" {
			rows = append(rows, e.Payload)
		}
		lastLSN = e.LSN
	}

	if len(rows) > 0 {
		if err := b.drv.WriteBatch(ctx, table, rows); err != nil {
			_ = b.store.RecordError(ctx, table, batchID, err.Error())
			return fmt.Errorf("write batch %s: %w", batchID, err)
		}
	}

	if lastLSN != "" {
		_ = b.store.UpdateWatermark(ctx, table, lastLSN, int64(len(rows)))
	}

	slog.Info("flushed batch", "table", table, "rows", len(rows), "lsn", lastLSN)
	return nil
}
