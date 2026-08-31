package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-warehouse/driver"
)

// BackfillRequest specifies one full-table backfill operation.
type BackfillRequest struct {
	Table string
	Since *time.Time // optional: only rows with updated_at > Since
}

// Backfiller reads directly from Postgres in pages and writes to the warehouse.
// It does NOT use the CDC stream — this avoids backpressure on the live pipeline.
type Backfiller struct {
	pool  *pgxpool.Pool
	drv   driver.Driver
	store *Store
	cfg   *Config
}

func newBackfiller(pool *pgxpool.Pool, drv driver.Driver, store *Store, cfg *Config) *Backfiller {
	return &Backfiller{pool: pool, drv: drv, store: store, cfg: cfg}
}

const pageSize = 500 // rows per SELECT page — conservative for OOM safety

// Run performs a full or incremental backfill for one table.
// Pages through the source table using CTID-based pagination to avoid
// loading the entire table into memory (OOM-safe for 1M+ rows).
func (bf *Backfiller) Run(ctx context.Context, req BackfillRequest) error {
	slog.Info("backfill started", "table", req.Table, "since", req.Since)

	var total int64
	offset := 0

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		rows, err := bf.fetchPage(ctx, req.Table, req.Since, offset)
		if err != nil {
			return fmt.Errorf("fetch page offset=%d: %w", offset, err)
		}
		if len(rows) == 0 {
			break
		}

		batchID := fmt.Sprintf("backfill-%s-%d", req.Table, time.Now().UnixNano())
		if err := bf.drv.WriteBatch(ctx, req.Table, rows); err != nil {
			_ = bf.store.RecordError(ctx, req.Table, batchID, err.Error())
			return fmt.Errorf("write batch: %w", err)
		}
		_ = bf.store.UpdateWatermark(ctx, req.Table, "backfill", int64(len(rows)))

		total += int64(len(rows))
		slog.Info("backfill progress", "table", req.Table, "rows_so_far", total)

		if len(rows) < pageSize {
			break // last page
		}
		offset += pageSize
	}

	slog.Info("backfill complete", "table", req.Table, "total_rows", total)
	return nil
}

// fetchPage returns one page of rows from the source np_* table.
// If since is non-nil, filters by updated_at (column must exist).
func (bf *Backfiller) fetchPage(
	ctx context.Context,
	table string,
	since *time.Time,
	offset int,
) ([]map[string]interface{}, error) {
	var query string
	var args []interface{}

	if since != nil {
		// #nosec G201 — table is validated against np_* allowlist before reaching here
		query = fmt.Sprintf(
			"SELECT * FROM %s WHERE updated_at > $1 ORDER BY updated_at LIMIT %d OFFSET %d",
			table, pageSize, offset,
		)
		args = []interface{}{*since}
	} else {
		// #nosec G201
		query = fmt.Sprintf(
			"SELECT * FROM %s ORDER BY ctid LIMIT %d OFFSET %d",
			table, pageSize, offset,
		)
	}

	pgRows, err := bf.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer pgRows.Close()

	fields := pgRows.FieldDescriptions()
	var result []map[string]interface{}

	for pgRows.Next() {
		values, err := pgRows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(fields))
		for i, f := range fields {
			row[string(f.Name)] = values[i]
		}
		result = append(result, row)
	}
	return result, pgRows.Err()
}
