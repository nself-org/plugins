package replication

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-cdc/broker"
	"github.com/nself-org/nself-cdc/router"
)

const snapshotBatchSize = 1000

// SnapshotWorker captures all existing rows for a table and streams them to the
// broker before live CDC begins. Ordering is by primary key ascending.
type SnapshotWorker struct {
	pool   *pgxpool.Pool
	router *router.TopicRouter
	broker broker.Broker
}

// NewSnapshotWorker creates a SnapshotWorker.
func NewSnapshotWorker(pool *pgxpool.Pool, tr *router.TopicRouter, brk broker.Broker) *SnapshotWorker {
	return &SnapshotWorker{pool: pool, router: tr, broker: brk}
}

// Snapshot exports all rows of tableName to the broker.
// Rows are published as synthetic INSERT events with op="c" and before=null.
func (sw *SnapshotWorker) Snapshot(ctx context.Context, tableName string) error {
	slog.Info("cdc: snapshot starting", "table", tableName)
	start := time.Now()
	total := 0

	tx, err := sw.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("snapshot: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ"); err != nil {
		return fmt.Errorf("snapshot: isolation: %w", err)
	}

	rows, err := tx.Query(ctx, fmt.Sprintf(
		`SELECT row_to_json(t) FROM (SELECT * FROM %s ORDER BY 1) t`, tableName))
	if err != nil {
		return fmt.Errorf("snapshot: query: %w", err)
	}
	defer rows.Close()

	var batch []*broker.Event
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := sw.broker.Publish(ctx, batch); err != nil {
			return fmt.Errorf("snapshot: publish: %w", err)
		}
		total += len(batch)
		batch = batch[:0]
		return nil
	}

	_ = sw.router.Topic(tableName, "INSERT") // topic wired in broker adapter
	for rows.Next() {
		var rawJSON json.RawMessage
		if err := rows.Scan(&rawJSON); err != nil {
			return fmt.Errorf("snapshot: scan: %w", err)
		}
		env := DebeziumEnvelope{
			Op:    "c",
			After: rawJSON,
			TsMs:  time.Now().UnixMilli(),
		}
		env.Source.Table = tableName
		env.Source.LSN = "snapshot"
		payload, _ := json.Marshal(env)

		ev := &broker.Event{
			Table:      tableName,
			Operation:  "INSERT",
			LSN:        "snapshot",
			Payload:    json.RawMessage(payload),
			OccurredAt: time.Now(),
		}
		batch = append(batch, ev)
		if len(batch) >= snapshotBatchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if rows.Err() != nil {
		return fmt.Errorf("snapshot: rows: %w", rows.Err())
	}
	if err := flush(); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("snapshot: commit: %w", err)
	}

	slog.Info("cdc: snapshot complete", "table", tableName, "rows", total,
		"elapsed_ms", time.Since(start).Milliseconds())
	return nil
}
