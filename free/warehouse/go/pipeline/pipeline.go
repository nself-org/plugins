package pipeline

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-warehouse/driver"
	"github.com/nself-org/nself-warehouse/driver/bigquery"
	"github.com/nself-org/nself-warehouse/driver/clickhouse"
	"github.com/nself-org/nself-warehouse/driver/snowflake"
)

// Pipeline ties together the consumer, batcher, backfiller, and store.
type Pipeline struct {
	cfg        *Config
	pool       *pgxpool.Pool
	drv        driver.Driver
	store      *Store
	batcher    *Batcher
	consumer   *Consumer
	backfiller *Backfiller
	paused     bool
}

// New constructs and connects all pipeline components.
func New(ctx context.Context, cfg *Config) (*Pipeline, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}

	drv, err := buildDriver(ctx, cfg)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("driver(%s): %w", cfg.Driver, err)
	}

	store := newStore(pool)
	batcher := newBatcher(cfg, drv, store)
	consumer := newConsumer(cfg, batcher)
	backfiller := newBackfiller(pool, drv, store, cfg)

	return &Pipeline{
		cfg:        cfg,
		pool:       pool,
		drv:        drv,
		store:      store,
		batcher:    batcher,
		consumer:   consumer,
		backfiller: backfiller,
	}, nil
}

// Run starts the streaming pipeline (consumer + flush loop). Blocks until ctx is cancelled.
func (p *Pipeline) Run(ctx context.Context) error {
	go p.batcher.RunFlushLoop(ctx)
	return p.consumer.Run(ctx)
}

// Close shuts down the pool and driver.
func (p *Pipeline) Close() {
	p.pool.Close()
	_ = p.drv.Close()
}

// Pause suspends event consumption (batcher keeps flushing in-flight batches).
func (p *Pipeline) Pause() { p.paused = true }

// Resume re-enables event consumption.
func (p *Pipeline) Resume() { p.paused = false }

// IsPaused returns the current pause state.
func (p *Pipeline) IsPaused() bool { return p.paused }

// Store exposes the watermark store for the API layer.
func (p *Pipeline) Store() *Store { return p.store }

// Backfiller exposes the backfill worker for the API layer.
func (p *Pipeline) Backfiller() *Backfiller { return p.backfiller }

// buildDriver constructs the target warehouse driver from config.
func buildDriver(ctx context.Context, cfg *Config) (driver.Driver, error) {
	switch cfg.Driver {
	case "clickhouse":
		return clickhouse.New(ctx, cfg.ClickhouseDSN)
	case "bigquery":
		return bigquery.New(ctx, bigquery.Config{
			Project:     cfg.BQProject,
			Dataset:     cfg.BQDataset,
			Credentials: cfg.BQCredentials,
		})
	case "snowflake":
		return snowflake.New(ctx, cfg.SnowflakeDSN)
	default:
		return nil, fmt.Errorf("unknown driver %q (must be clickhouse|bigquery|snowflake)", cfg.Driver)
	}
}
