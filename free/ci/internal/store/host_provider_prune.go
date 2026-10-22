package store

import (
	"context"
	"database/sql"
	"time"
)

// ProviderMarkPrune removes marks strictly older than the cutoff for one namespace.
func (s *Store) ProviderMarkPrune(ctx context.Context, name string, olderThan time.Time) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM provider_mark WHERE name=? AND at<?", name, olderThan.UnixNano())
		return err
	})
}

// PruneReconcileMarks retains reconcile history for 30 days and volatile run/demand marks for 7.
func (s *Store) PruneReconcileMarks(ctx context.Context, now time.Time) error {
	if err := s.ProviderMarkPrune(ctx, "github-reconcile", now.Add(-30*24*time.Hour)); err != nil {
		return err
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM provider_mark WHERE name='github-reconcile' AND at<? AND (substr(key,1,4)='run|' OR substr(key,1,7)='demand|')`, now.Add(-7*24*time.Hour).UnixNano())
		return err
	})
}
