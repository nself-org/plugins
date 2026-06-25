package internal

import (
	pgx "github.com/jackc/pgx/v5"
	"context"
	"fmt"
	"time"
)

// stale_after_days is set). These are candidates for pruning / archival review.
func (d *DB) ListStaleFlags(ctx context.Context) ([]Flag, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT `+flagColumns+`
		 FROM np_feature_flags_flags
		 WHERE stale_after_days IS NOT NULL
		   AND updated_at < NOW() - (stale_after_days || ' days')::INTERVAL
		 ORDER BY updated_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list stale flags: %w", err)
	}
	defer rows.Close()

	var flags []Flag
	for rows.Next() {
		var f Flag
		if err := scanFlag(rows, &f); err != nil {
			return nil, fmt.Errorf("scan stale flag: %w", err)
		}
		flags = append(flags, f)
	}
	return flags, rows.Err()
}

// --- Segment CRUD ---

// GetSegment returns a segment by ID.
func (d *DB) GetSegment(id string) (*Segment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var s Segment
	err := d.pool.QueryRow(ctx,
		`SELECT id, name, rules, created_at, updated_at
		 FROM np_feature_flags_segments WHERE id = $1`, id,
	).Scan(&s.ID, &s.Name, &s.Rules, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get segment: %w", err)
	}
	return &s, nil
}

// joinStrings joins a slice of strings with a separator (avoids importing strings).
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += sep + p
	}
	return result
}
