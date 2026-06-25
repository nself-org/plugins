package internal

import (
	pgx "github.com/jackc/pgx/v5"
	"fmt"
)

// =========================================================================
// Helpers
// =========================================================================

// scanPositions scans rows into a slice of ProgressPosition.
func scanPositions(rows pgx.Rows) ([]ProgressPosition, error) {
	var positions []ProgressPosition
	for rows.Next() {
		var pos ProgressPosition
		if err := rows.Scan(
			&pos.ID, &pos.SourceAccountID, &pos.UserID, &pos.ContentType, &pos.ContentID,
			&pos.PositionSeconds, &pos.DurationSeconds, &pos.ProgressPercent,
			&pos.Completed, &pos.CompletedAt, &pos.DeviceID, &pos.AudioTrack, &pos.SubtitleTrack,
			&pos.Quality, &pos.Metadata, &pos.UpdatedAt, &pos.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		positions = append(positions, pos)
	}
	if positions == nil {
		positions = []ProgressPosition{}
	}
	return positions, rows.Err()
}

