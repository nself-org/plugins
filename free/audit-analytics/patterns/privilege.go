package patterns

import (
	"context"
	"fmt"
	"time"
)

// PrivilegedReview is a single row from np_audit_privileged_reviews.
type PrivilegedReview struct {
	ID         string     `json:"id"`
	TenantID   *string    `json:"tenant_id,omitempty"`
	AuditLogID string     `json:"audit_log_id"`
	Action     string     `json:"action"`
	ActorID    string     `json:"actor_id"`
	DueAt      time.Time  `json:"due_at"`
	ReviewerID *string    `json:"reviewer_id,omitempty"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	Notes      *string    `json:"notes,omitempty"`
	Overdue    bool       `json:"overdue"`
}

// ListPendingReviews returns privileged action reviews that have not yet been reviewed.
// tenantID filters to a specific Cloud tenant; empty means self-host (no filter).
func ListPendingReviews(ctx context.Context, query QueryFn, tenantID string) ([]PrivilegedReview, error) {
	var (
		rows []map[string]interface{}
		err  error
	)
	if tenantID != "" {
		rows, err = query(ctx, `
			SELECT id, tenant_id, audit_log_id, action, actor_id, due_at,
			       reviewer_id, reviewed_at, notes,
			       NOW() > due_at AS overdue
			  FROM np_audit_privileged_reviews
			 WHERE reviewed_at IS NULL
			   AND tenant_id = $1
			 ORDER BY due_at ASC
		`, tenantID)
	} else {
		rows, err = query(ctx, `
			SELECT id, tenant_id, audit_log_id, action, actor_id, due_at,
			       reviewer_id, reviewed_at, notes,
			       NOW() > due_at AS overdue
			  FROM np_audit_privileged_reviews
			 WHERE reviewed_at IS NULL
			 ORDER BY due_at ASC
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("pending reviews query: %w", err)
	}
	return parseReviewRows(rows), nil
}

// ListOverdueReviews returns reviews past their due_at that are still unreviewed.
// tenantID filters to a specific Cloud tenant; empty means self-host (no filter).
func ListOverdueReviews(ctx context.Context, query QueryFn, tenantID string) ([]PrivilegedReview, error) {
	var (
		rows []map[string]interface{}
		err  error
	)
	if tenantID != "" {
		rows, err = query(ctx, `
			SELECT id, tenant_id, audit_log_id, action, actor_id, due_at,
			       reviewer_id, reviewed_at, notes, TRUE AS overdue
			  FROM np_audit_privileged_reviews
			 WHERE reviewed_at IS NULL
			   AND NOW() > due_at
			   AND tenant_id = $1
			 ORDER BY due_at ASC
		`, tenantID)
	} else {
		rows, err = query(ctx, `
			SELECT id, tenant_id, audit_log_id, action, actor_id, due_at,
			       reviewer_id, reviewed_at, notes, TRUE AS overdue
			  FROM np_audit_privileged_reviews
			 WHERE reviewed_at IS NULL
			   AND NOW() > due_at
			 ORDER BY due_at ASC
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("overdue reviews query: %w", err)
	}
	return parseReviewRows(rows), nil
}

// AnnotateReview marks a review as completed with reviewer and notes.
// tenantID scopes the UPDATE to the caller's tenant when non-empty.
func AnnotateReview(ctx context.Context, exec ExecFn, reviewID, reviewerID, notes, tenantID string) error {
	var (
		n   int64
		err error
	)
	if tenantID != "" {
		n, err = exec(ctx, `
			UPDATE np_audit_privileged_reviews
			   SET reviewer_id = $1,
			       reviewed_at = NOW(),
			       notes = $2
			 WHERE id = $3
			   AND tenant_id = $4
			   AND reviewed_at IS NULL
		`, reviewerID, notes, reviewID, tenantID)
	} else {
		n, err = exec(ctx, `
			UPDATE np_audit_privileged_reviews
			   SET reviewer_id = $1,
			       reviewed_at = NOW(),
			       notes = $2
			 WHERE id = $3
			   AND reviewed_at IS NULL
		`, reviewerID, notes, reviewID)
	}
	if err != nil {
		return fmt.Errorf("annotate review: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("review %s not found or already reviewed", reviewID)
	}
	return nil
}

func parseReviewRows(rows []map[string]interface{}) []PrivilegedReview {
	out := make([]PrivilegedReview, 0, len(rows))
	for _, row := range rows {
		r := PrivilegedReview{
			ID:         fmt.Sprintf("%v", row["id"]),
			AuditLogID: fmt.Sprintf("%v", row["audit_log_id"]),
			Action:     fmt.Sprintf("%v", row["action"]),
			ActorID:    fmt.Sprintf("%v", row["actor_id"]),
		}
		if tid, ok := row["tenant_id"]; ok && tid != nil {
			s := fmt.Sprintf("%v", tid)
			r.TenantID = &s
		}
		if da, ok := row["due_at"]; ok {
			if t, ok := da.(time.Time); ok {
				r.DueAt = t
			}
		}
		if ra, ok := row["reviewed_at"]; ok && ra != nil {
			if t, ok := ra.(time.Time); ok {
				r.ReviewedAt = &t
			}
		}
		if rid, ok := row["reviewer_id"]; ok && rid != nil {
			s := fmt.Sprintf("%v", rid)
			r.ReviewerID = &s
		}
		if n, ok := row["notes"]; ok && n != nil {
			s := fmt.Sprintf("%v", n)
			r.Notes = &s
		}
		if ov, ok := row["overdue"]; ok {
			r.Overdue, _ = ov.(bool)
		}
		out = append(out, r)
	}
	return out
}
