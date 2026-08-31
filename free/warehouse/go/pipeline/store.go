package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps the np_warehouse_watermarks and np_warehouse_errors tables.
type Store struct {
	pool *pgxpool.Pool
}

func newStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// UpdateWatermark upserts the export watermark for a table.
func (s *Store) UpdateWatermark(ctx context.Context, table, lsn string, rowCount int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO np_warehouse_watermarks (table_name, last_lsn, exported_at, row_count)
		VALUES ($1, $2, now(), $3)
		ON CONFLICT (table_name)
		DO UPDATE SET last_lsn = EXCLUDED.last_lsn,
		              exported_at = EXCLUDED.exported_at,
		              row_count = np_warehouse_watermarks.row_count + EXCLUDED.row_count
	`, table, lsn, rowCount)
	return err
}

// RecordError persists an export error to np_warehouse_errors.
func (s *Store) RecordError(ctx context.Context, table, batchID, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO np_warehouse_errors (table_name, batch_id, error, occurred_at)
		VALUES ($1, $2, $3, now())
	`, table, batchID, errMsg)
	return err
}

// WatermarkRow is the status for one table returned by the status endpoint.
type WatermarkRow struct {
	TableName  string    `json:"table_name"`
	LastLSN    string    `json:"last_lsn"`
	ExportedAt time.Time `json:"exported_at"`
	RowCount   int64     `json:"row_count"`
}

// AllWatermarks returns all watermark rows ordered by table name.
func (s *Store) AllWatermarks(ctx context.Context) ([]WatermarkRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT table_name, last_lsn, exported_at, row_count
		FROM np_warehouse_watermarks
		ORDER BY table_name
	`)
	if err != nil {
		return nil, fmt.Errorf("query watermarks: %w", err)
	}
	defer rows.Close()

	var result []WatermarkRow
	for rows.Next() {
		var r WatermarkRow
		if err := rows.Scan(&r.TableName, &r.LastLSN, &r.ExportedAt, &r.RowCount); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// RecentErrors returns the 50 most recent export errors.
func (s *Store) RecentErrors(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, table_name, batch_id, error, occurred_at
		FROM np_warehouse_errors
		ORDER BY occurred_at DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, fmt.Errorf("query errors: %w", err)
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id, table, batch, errMsg string
		var at time.Time
		if err := rows.Scan(&id, &table, &batch, &errMsg, &at); err != nil {
			return nil, err
		}
		result = append(result, map[string]interface{}{
			"id": id, "table_name": table, "batch_id": batch,
			"error": errMsg, "occurred_at": at,
		})
	}
	return result, rows.Err()
}
