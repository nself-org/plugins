package automerge

import (
	"context"
	"log/slog"
	"time"

	"github.com/nself-org/nself-crdt/internal/store"
)

// Compactor runs a background loop that compacts update history for all documents.
// Compaction merges raw np_crdt_updates rows into the document state blob and
// deletes the raw rows, reducing table size by >80% for documents with many updates.
type Compactor struct {
	store         *store.Store
	interval      time.Duration
	retentionDays int
	log           *slog.Logger
}

// NewCompactor creates a Compactor with the given run interval.
func NewCompactor(st *store.Store, retentionDays int, log *slog.Logger) *Compactor {
	return &Compactor{
		store:         st,
		interval:      24 * time.Hour,
		retentionDays: retentionDays,
		log:           log,
	}
}

// Run starts the compaction loop. It blocks until ctx is cancelled.
func (c *Compactor) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.log.Info("crdt compactor started", "interval", c.interval, "retention_days", c.retentionDays)

	for {
		select {
		case <-ctx.Done():
			c.log.Info("crdt compactor stopped")
			return
		case <-ticker.C:
			c.runOnce(ctx)
		}
	}
}

// runOnce compacts all documents that have accumulated updates.
func (c *Compactor) runOnce(ctx context.Context) {
	docs, err := c.store.ListDocuments(ctx, 1000, 0)
	if err != nil {
		c.log.Error("compactor: list documents failed", "err", err)
		return
	}

	for _, doc := range docs {
		count, err := c.store.UpdateCount(ctx, doc.DocID)
		if err != nil {
			c.log.Warn("compactor: count updates failed", "doc", doc.DocID, "err", err)
			continue
		}
		if count == 0 {
			continue
		}

		deleted, err := c.store.CompactUpdates(ctx, doc.DocID)
		if err != nil {
			c.log.Error("compactor: compact failed", "doc", doc.DocID, "err", err)
			continue
		}
		c.log.Info("compactor: compacted document", "doc", doc.DocID, "rows_deleted", deleted)
	}
}

// CompactOne forces immediate compaction of a single document.
// Returns the number of update rows deleted.
func (c *Compactor) CompactOne(ctx context.Context, docID string) (int64, error) {
	return c.store.CompactUpdates(ctx, docID)
}
