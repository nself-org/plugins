// reader.go — reads rows from np_audit_log since a given cursor.
package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadAuditRows fetches at most limit rows from np_audit_log created after since.
// Returns rows in ascending created_at order so the caller can advance the cursor.
func ReadAuditRows(ctx context.Context, pool *pgxpool.Pool, since time.Time, limit int) ([]AuditRow, error) {
	const q = `
		SELECT
			id::text,
			tenant_id::text,
			action,
			table_name,
			COALESCE(record_id::text, '') AS record_id,
			COALESCE(user_id::text, '') AS user_id,
			COALESCE((session_data->>'user_name')::text, '') AS user_name,
			COALESCE((session_data->>'ip_address')::text, '') AS ip_address,
			created_at
		FROM np_audit_log
		WHERE created_at > $1
		ORDER BY created_at ASC
		LIMIT $2
	`
	rows, err := pool.Query(ctx, q, since, limit)
	if err != nil {
		return nil, fmt.Errorf("ReadAuditRows query: %w", err)
	}
	defer rows.Close()

	var out []AuditRow
	for rows.Next() {
		var r AuditRow
		var tenantID *string
		if err := rows.Scan(
			&r.ID,
			&tenantID,
			&r.Action,
			&r.TableName,
			&r.RecordID,
			&r.UserID,
			&r.UserName,
			&r.IPAddress,
			&r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("ReadAuditRows scan: %w", err)
		}
		r.TenantID = tenantID
		out = append(out, r)
	}
	return out, rows.Err()
}
