package internal

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GetEvent(ctx context.Context, pool *pgxpool.Pool, id string) (*AuditEvent, error) {
	e := &AuditEvent{}
	err := pool.QueryRow(ctx, `
		SELECT id, source_account_id, actor_user_id, actor_type, event_type,
		       resource_type, resource_id, ip_address, user_agent, metadata,
		       severity, source_plugin, target_plugin, created_at
		FROM np_auditlog_events
		WHERE id = $1
		LIMIT 1
	`, id).Scan(
		&e.ID,
		&e.SourceAccountID,
		&e.ActorUserID,
		&e.ActorType,
		&e.EventType,
		&e.ResourceType,
		&e.ResourceID,
		&e.IPAddress,
		&e.UserAgent,
		&e.Metadata,
		&e.Severity,
		&e.SourcePlugin,
		&e.TargetPlugin,
		&e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return e, nil
}
