package internal

import (
	"context"
	"encoding/json"
	pgx "github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Leak test operations
// ---------------------------------------------------------------------------

// GetLatestLeakTest returns the most recent leak test for a connection.
func (d *DB) GetLatestLeakTest(ctx context.Context, connectionID string) (*LeakTest, error) {
	var lt LeakTest
	var detailsJSON []byte
	err := d.pool.QueryRow(ctx,
		`SELECT id, connection_id, test_type, passed, expected_value, actual_value,
			details, tested_at, source_account_id
		FROM np_vpn_leak_tests
		WHERE connection_id = $1 AND source_account_id = $2
		ORDER BY tested_at DESC LIMIT 1`,
		connectionID, d.sourceAccountID,
	).Scan(
		&lt.ID, &lt.ConnectionID, &lt.TestType, &lt.Passed,
		&lt.ExpectedValue, &lt.ActualValue, &detailsJSON,
		&lt.TestedAt, &lt.SourceAccountID,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lt.Details = make(map[string]interface{})
	if len(detailsJSON) > 0 {
		_ = json.Unmarshal(detailsJSON, &lt.Details)
	}
	return &lt, nil
}

// InsertLeakTest stores a leak test result.
func (d *DB) InsertLeakTest(ctx context.Context, connectionID, testType string, passed bool, expected, actual string, details map[string]interface{}) error {
	detailsJSON, _ := json.Marshal(details)
	_, err := d.pool.Exec(ctx,
		`INSERT INTO np_vpn_leak_tests (connection_id, test_type, passed, expected_value, actual_value, details, source_account_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		connectionID, testType, passed, expected, actual, detailsJSON, d.sourceAccountID)
	return err
}

