package store

import (
	"context"
	"database/sql"
)

// NodeAgeRecipient reads the durable hello pin, independent of capability refreshes.
func (s *Store) NodeAgeRecipient(ctx context.Context, id string) (*string, error) {
	var pin *string
	err := s.readers.QueryRowContext(ctx, `SELECT age_recipient FROM node WHERE id=?`, id).Scan(&pin)
	return pin, err
}

// NodePinAgeRecipient pins once and returns the winning value under concurrent hellos.
func (s *Store) NodePinAgeRecipient(ctx context.Context, id, presented string) (string, error) {
	var pin string
	err := s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE node SET age_recipient=? WHERE id=? AND age_recipient IS NULL`, presented, id); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `SELECT age_recipient FROM node WHERE id=?`, id).Scan(&pin)
	})
	return pin, err
}
