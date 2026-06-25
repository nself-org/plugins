package internal

import (
	"context"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
)

func InsertEvent(ctx context.Context, pool *pgxpool.Pool, e *AuditEvent) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_auditlog_events
			(id, source_account_id, actor_user_id, actor_type, event_type,
			 resource_type, resource_id, ip_address, user_agent, metadata,
			 severity, source_plugin, target_plugin, created_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`,
		e.ID,
		e.SourceAccountID,
		e.ActorUserID,
		e.ActorType,
		e.EventType,
		e.ResourceType,
		e.ResourceID,
		e.IPAddress,
		e.UserAgent,
		e.Metadata,
		e.Severity,
		e.SourcePlugin,
		e.TargetPlugin,
		e.CreatedAt,
	)
	return err
}

// QueryFilter holds the optional filter parameters for ListEvents and ExportEvents.
type QueryFilter struct {
	EventType       string
	ActorUserID     string
	Severity        string
	SourceAccountID string
	From            *time.Time
	To              *time.Time
	Limit           int
	Offset          int
}

// ListEvents returns audit events matching the given filter, ordered by
// created_at DESC. Pagination is via limit/offset.
