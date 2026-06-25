package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// =========================================================================
// Download Rules
// =========================================================================

const ruleColumns = `id, source_account_id, user_id, name, conditions, action,
  priority, enabled, created_at, updated_at`

func scanRule(row pgx.Row) (*DownloadRule, error) {
	var r DownloadRule
	err := row.Scan(
		&r.ID, &r.SourceAccountID, &r.UserID, &r.Name, &r.Conditions,
		&r.Action, &r.Priority, &r.Enabled, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateDownloadRule inserts a new download rule.
func (d *DB) CreateDownloadRule(accountID, name string, conditions json.RawMessage, action string, priority int, enabled bool) (*DownloadRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_download_rules
			   (source_account_id, user_id, name, conditions, action, priority, enabled)
			 VALUES ($1, $1, $2, $3, $4, $5, $6)
			 RETURNING %s`, ruleColumns),
		accountID, name, conditions, action, priority, enabled,
	)
	return scanRule(row)
}

// GetDownloadRule returns a single rule by ID.
func (d *DB) GetDownloadRule(id string) (*DownloadRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_download_rules WHERE id = $1`, ruleColumns), id)
	r, err := scanRule(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// ListDownloadRules returns all rules for an account.
func (d *DB) ListDownloadRules(accountID string) ([]DownloadRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		fmt.Sprintf(
			`SELECT %s FROM np_contentacquisition_download_rules
			 WHERE source_account_id = $1
			 ORDER BY priority DESC, created_at DESC`, ruleColumns),
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []DownloadRule
	for rows.Next() {
		var r DownloadRule
		if err := rows.Scan(
			&r.ID, &r.SourceAccountID, &r.UserID, &r.Name, &r.Conditions,
			&r.Action, &r.Priority, &r.Enabled, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// UpdateDownloadRule updates allowed fields on a download rule.
// Size-cap exception: single DB operation — 52L scan loop with struct mapping; splitting would fragment a single SQL query across files.
func (d *DB) UpdateDownloadRule(id string, req UpdateRuleRequest) (*DownloadRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", idx))
		args = append(args, *req.Name)
		idx++
	}
	if req.Conditions != nil {
		setClauses = append(setClauses, fmt.Sprintf("conditions = $%d", idx))
		args = append(args, *req.Conditions)
		idx++
	}
	if req.Action != nil {
		setClauses = append(setClauses, fmt.Sprintf("action = $%d", idx))
		args = append(args, *req.Action)
		idx++
	}
	if req.Priority != nil {
		setClauses = append(setClauses, fmt.Sprintf("priority = $%d", idx))
		args = append(args, *req.Priority)
		idx++
	}
	if req.Enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", idx))
		args = append(args, *req.Enabled)
		idx++
	}

	if len(args) == 0 {
		return d.GetDownloadRule(id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE np_contentacquisition_download_rules SET %s WHERE id = $%d
		 RETURNING %s`,
		strings.Join(setClauses, ", "), idx, ruleColumns,
	)

	row := d.pool.QueryRow(ctx, query, args...)
	r, err := scanRule(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// DeleteDownloadRule deletes a rule by ID.
func (d *DB) DeleteDownloadRule(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_contentacquisition_download_rules WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

