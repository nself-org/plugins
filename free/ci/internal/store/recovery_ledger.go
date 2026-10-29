package store

import "context"

// UnsettledDeadRuns includes earlier recoveries whose settlement was interrupted.
func (s *Store) UnsettledDeadRuns(ctx context.Context) ([]string, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT DISTINCT p.id FROM pipeline p JOIN ledger l ON l.pipeline_id=p.id WHERE p.status='lost' AND l.finalized_at IS NULL ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
