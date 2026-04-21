package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// AuditEntry represents a row in np_feature_flags_audit.
type AuditEntry struct {
	ID      string          `json:"id"`
	FlagKey string          `json:"flag_key"`
	Actor   string          `json:"actor"`
	Action  string          `json:"action"`
	Before  json.RawMessage `json:"before"`
	After   json.RawMessage `json:"after"`
	Reason  *string         `json:"reason"`
	Ts      time.Time       `json:"ts"`
}

// WriteAudit inserts an audit row for a flag state change.
// before and after should be the JSON-encoded complete flag states before and
// after the mutation. reason is optional (required for kill; nullable otherwise).
func (d *DB) WriteAudit(ctx context.Context, flagKey, actor, action string, before, after json.RawMessage, reason *string) error {
	_, err := d.pool.Exec(ctx,
		`INSERT INTO np_feature_flags_audit (flag_key, actor, action, before, after, reason)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		flagKey, actor, action, before, after, reason,
	)
	if err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	return nil
}

// ListAudit returns audit entries ordered newest first.
// When flagKey is non-empty, filters to that flag only.
// When flagKey is empty, returns entries across all flags.
func (d *DB) ListAudit(ctx context.Context, flagKey string, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	var (
		rows interface {
			Next() bool
			Scan(...interface{}) error
			Close()
			Err() error
		}
		err error
	)

	if flagKey == "" {
		rows, err = d.pool.Query(ctx,
			`SELECT id, flag_key, actor, action, before, after, reason, ts
			 FROM np_feature_flags_audit
			 ORDER BY ts DESC
			 LIMIT $1`,
			limit,
		)
	} else {
		rows, err = d.pool.Query(ctx,
			`SELECT id, flag_key, actor, action, before, after, reason, ts
			 FROM np_feature_flags_audit
			 WHERE flag_key = $1
			 ORDER BY ts DESC
			 LIMIT $2`,
			flagKey, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.FlagKey, &e.Actor, &e.Action, &e.Before, &e.After, &e.Reason, &e.Ts); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// marshalFlag returns a JSON-encoded snapshot of a flag (for before/after audit).
func marshalFlag(f *Flag) json.RawMessage {
	if f == nil {
		return json.RawMessage("null")
	}
	b, err := json.Marshal(f)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(b)
}
