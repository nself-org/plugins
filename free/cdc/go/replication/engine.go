package replication

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-cdc/broker"
	"github.com/nself-org/nself-cdc/router"
)

// hardStopBufferDepth pauses the WAL reader when np_cdc_events exceeds this many unbrokered rows.
const hardStopBufferDepth = 100_000

// EngineConfig holds all parameters needed to start the replication engine.
type EngineConfig struct {
	DatabaseURL     string
	SlotName        string
	PublicationName string
	Tables          []string // empty = all np_*
	BatchSize       int
	FlushMS         int
	Mode            string // embedded | debezium-connect
	DebeziumURL     string
}

// Engine orchestrates the WAL reader, broker adapter, and buffer drain.
type Engine struct {
	cfg    EngineConfig
	pool   *pgxpool.Pool
	slot   *SlotReader
	snap   *SnapshotWorker
	router *router.TopicRouter
	broker broker.Broker
	paused atomic.Bool
	mu     sync.Mutex

	eventsPS atomic.Int64 // events per second, updated every 5s
	lagBytes atomic.Int64

	running atomic.Bool
}

// NewEngine builds and wires the Engine. Does not start streaming.
func NewEngine(ctx context.Context, cfg EngineConfig, tr *router.TopicRouter, brk broker.Broker) (*Engine, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("engine: pool: %w", err)
	}

	dec := NewDecoder()
	slot, err := NewSlotReader(ctx, cfg, dec)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("engine: slot reader: %w", err)
	}

	snap := NewSnapshotWorker(pool, tr, brk)

	return &Engine{
		cfg:    cfg,
		pool:   pool,
		slot:   slot,
		snap:   snap,
		router: tr,
		broker: brk,
	}, nil
}

// Run starts the streaming loop. Blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	e.running.Store(true)
	defer e.running.Store(false)

	// First: drain any buffered unbrokered events from prior runs.
	if err := e.drainBuffer(ctx); err != nil {
		slog.Warn("cdc: drain buffer on start", "err", err)
	}

	// Start WAL reader.
	ch, err := e.slot.Start(ctx)
	if err != nil {
		return fmt.Errorf("engine: start replication: %w", err)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var batch []*broker.Event
	flushTimer := time.NewTimer(time.Duration(e.cfg.FlushMS) * time.Millisecond)
	defer flushTimer.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := e.publishOrBuffer(ctx, batch); err != nil {
			slog.Error("cdc: flush", "err", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return nil

		case ev, ok := <-ch:
			if !ok {
				flush()
				return fmt.Errorf("engine: replication channel closed")
			}
			if e.paused.Load() {
				if err := e.writeBuffer(ctx, []*broker.Event{ev}); err != nil {
					slog.Error("cdc: paused buffer write", "err", err)
				}
				continue
			}
			batch = append(batch, ev)
			if len(batch) >= e.cfg.BatchSize {
				flush()
				if !flushTimer.Stop() {
					select {
					case <-flushTimer.C:
					default:
					}
				}
				flushTimer.Reset(time.Duration(e.cfg.FlushMS) * time.Millisecond)
			}

		case <-flushTimer.C:
			flush()
			flushTimer.Reset(time.Duration(e.cfg.FlushMS) * time.Millisecond)

		case <-ticker.C:
			e.updateMetrics(ctx)
		}
	}
}

// Snapshot triggers a full-table snapshot. Called by POST /cdc/snapshot.
func (e *Engine) Snapshot(ctx context.Context, table string) error {
	return e.snap.Snapshot(ctx, table)
}

// Pause stops sending events to the broker (slot stays open; events buffer).
func (e *Engine) Pause() { e.paused.Store(true) }

// Resume restarts sending events; drains the buffer first.
func (e *Engine) Resume(ctx context.Context) {
	e.paused.Store(false)
	go func() {
		if err := e.drainBuffer(ctx); err != nil {
			slog.Error("cdc: drain on resume", "err", err)
		}
	}()
}

// IsRunning returns true if the streaming loop is active.
func (e *Engine) IsRunning() bool { return e.running.Load() }

// IsPaused returns true if streaming is paused.
func (e *Engine) IsPaused() bool { return e.paused.Load() }

// SlotLagBytes returns the last observed replication slot lag in bytes.
func (e *Engine) SlotLagBytes() int64 { return e.lagBytes.Load() }

// EventsPerSecond returns the smoothed throughput over the last 5s window.
func (e *Engine) EventsPerSecond() int64 { return e.eventsPS.Load() }

// Close shuts down the engine.
func (e *Engine) Close() {
	e.slot.Close(context.Background())
	e.pool.Close()
}

func (e *Engine) publishOrBuffer(ctx context.Context, events []*broker.Event) error {
	if err := e.broker.Publish(ctx, events); err != nil {
		slog.Warn("cdc: broker publish failed, buffering", "count", len(events), "err", err)
		return e.writeBuffer(ctx, events)
	}
	return nil
}

func (e *Engine) writeBuffer(ctx context.Context, events []*broker.Event) error {
	for _, ev := range events {
		payload, _ := json.Marshal(ev.Payload)
		_, err := e.pool.Exec(ctx,
			`INSERT INTO np_cdc_events (table_name, operation, lsn, payload, brokered)
			 VALUES ($1, $2, $3, $4, false)`,
			ev.Table, ev.Operation, ev.LSN, payload)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) drainBuffer(ctx context.Context) error {
	for {
		rows, err := e.pool.Query(ctx,
			`SELECT id, table_name, operation, lsn, payload
			   FROM np_cdc_events WHERE brokered = false
			   ORDER BY occurred_at LIMIT $1`, e.cfg.BatchSize)
		if err != nil {
			return err
		}

		var ids []int64
		var events []*broker.Event
		for rows.Next() {
			var id int64
			ev := &broker.Event{}
			var payload json.RawMessage
			if err := rows.Scan(&id, &ev.Table, &ev.Operation, &ev.LSN, &payload); err != nil {
				rows.Close()
				return err
			}
			ev.Payload = payload
			ids = append(ids, id)
			events = append(events, ev)
		}
		rows.Close()
		if rows.Err() != nil {
			return rows.Err()
		}
		if len(events) == 0 {
			return nil
		}

		if err := e.broker.Publish(ctx, events); err != nil {
			return fmt.Errorf("drain: publish: %w", err)
		}

		for _, id := range ids {
			_, _ = e.pool.Exec(ctx, `UPDATE np_cdc_events SET brokered = true WHERE id = $1`, id)
		}

		slog.Info("cdc: drained buffer batch", "count", len(events))

		if len(events) < e.cfg.BatchSize {
			return nil
		}
	}
}

func (e *Engine) updateMetrics(ctx context.Context) {
	lag, err := e.slot.CurrentLag(ctx)
	if err == nil {
		e.lagBytes.Store(lag)
		if lag > 1<<30 {
			slog.Warn("cdc: slot lag exceeds 1 GB alert threshold", "lag_bytes", lag)
		}
	}

	var depth int64
	row := e.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_cdc_events WHERE brokered = false`)
	_ = row.Scan(&depth)
	if depth > 10_000 {
		slog.Warn("cdc: buffer depth warning", "depth", depth)
	}
	if depth > hardStopBufferDepth {
		slog.Error("cdc: buffer hard stop — pausing WAL reader", "depth", depth)
		e.paused.Store(true)
	}
}

// Migrate applies the CDC schema (slot, publication, np_cdc_events).
func Migrate(ctx context.Context, dbURL string) error {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("migrate: connect: %w", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, migrationSQL)
	return err
}

const migrationSQL = `
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = 'nself_cdc') THEN
    PERFORM pg_create_logical_replication_slot('nself_cdc', 'pgoutput');
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_publication WHERE pubname = 'nself_pub') THEN
    CREATE PUBLICATION nself_pub FOR ALL TABLES;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS np_cdc_events (
  id          BIGSERIAL    PRIMARY KEY,
  table_name  TEXT         NOT NULL,
  operation   TEXT         NOT NULL CHECK (operation IN ('INSERT','UPDATE','DELETE')),
  lsn         TEXT         NOT NULL,
  payload     JSONB        NOT NULL,
  brokered    BOOLEAN      NOT NULL DEFAULT false,
  occurred_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS np_cdc_events_brokered_occurred
  ON np_cdc_events (brokered, occurred_at);
`
