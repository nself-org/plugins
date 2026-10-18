package patterns

import (
	"context"
	"fmt"
)

// ActorSummary is a single user's action count in a time window.
type ActorSummary struct {
	UserID      string  `json:"user_id"`
	TenantID    *string `json:"tenant_id,omitempty"`
	ActionCount int64   `json:"action_count"`
}

// TopActors returns the N most active users in the last `days` days.
// tenantID filters to a specific Cloud tenant; empty means self-host (no filter).
func TopActors(ctx context.Context, query QueryFn, n, days int, tenantID string) ([]ActorSummary, error) {
	if n <= 0 {
		n = 20
	}
	if days <= 0 {
		days = 7
	}

	var (
		rows []map[string]interface{}
		err  error
	)
	if tenantID != "" {
		rows, err = query(ctx, `
			SELECT user_id, tenant_id, COUNT(*) AS action_count
			  FROM np_audit_log
			 WHERE created_at > NOW() - ($1 || ' days')::INTERVAL
			   AND tenant_id = $3
			 GROUP BY user_id, tenant_id
			 ORDER BY action_count DESC
			 LIMIT $2
		`, days, n, tenantID)
	} else {
		rows, err = query(ctx, `
			SELECT user_id, tenant_id, COUNT(*) AS action_count
			  FROM np_audit_log
			 WHERE created_at > NOW() - ($1 || ' days')::INTERVAL
			 GROUP BY user_id, tenant_id
			 ORDER BY action_count DESC
			 LIMIT $2
		`, days, n)
	}
	if err != nil {
		return nil, fmt.Errorf("top actors query: %w", err)
	}

	out := make([]ActorSummary, 0, len(rows))
	for _, row := range rows {
		a := ActorSummary{
			UserID:      fmt.Sprintf("%v", row["user_id"]),
			ActionCount: toInt64(row["action_count"]),
		}
		if tid, ok := row["tenant_id"]; ok && tid != nil {
			s := fmt.Sprintf("%v", tid)
			a.TenantID = &s
		}
		out = append(out, a)
	}
	return out, nil
}
