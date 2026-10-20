package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/nself-org/nself-byok/tenantdb"
)

// ──────────────────────────────────────────────────────────────────────────
// Key rotation handlers
// ──────────────────────────────────────────────────────────────────────────

// TriggerRotation starts a key rotation job (simplified — real impl uses async worker).
func (h *byokHandler) TriggerRotation(w http.ResponseWriter, r *http.Request) {
	tenantID, err := tenantIDFromRequest(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "tenant_id required")
		return
	}

	var keyRef string
	if err := tenantdb.With(r.Context(), h.db, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(r.Context(),
			`SELECT key_ref FROM np_kms_configs WHERE tenant_id = $1 AND enabled = TRUE`, tenantID,
		).Scan(&keyRef)
	}); err != nil {
		errJSON(w, http.StatusNotFound, "KMS not configured or disabled")
		return
	}

	jobID := uuid.New()
	h.logKeyEvent(r.Context(), tenantID, uuid.Nil, "rotation_started", keyRef, map[string]any{
		"job_id": jobID.String(),
	})

	writeJSON(w, http.StatusAccepted, map[string]string{
		"job_id": jobID.String(),
		"status": "started",
	})
}

// RotationStatus returns the status of a rotation job by ID.
func (h *byokHandler) RotationStatus(w http.ResponseWriter, r *http.Request) {
	jobIDStr := chi.URLParam(r, "job_id")
	if jobIDStr == "" {
		errJSON(w, http.StatusBadRequest, "job_id required")
		return
	}

	// A job id alone used to be enough; under RLS the tenant scopes the lookup, so one
	// tenant can never read another tenant's rotation events by guessing a job id.
	tenantID, err := tenantIDFromRequest(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "tenant_id required")
		return
	}

	var eventType string
	var details map[string]any
	var occurredAt time.Time
	if err := tenantdb.With(r.Context(), h.db, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(r.Context(),
			`SELECT event_type, details, occurred_at
		   FROM np_encryption_key_events
		  WHERE tenant_id = $2 AND details->>'job_id' = $1
		  ORDER BY occurred_at DESC LIMIT 1`, jobIDStr, tenantID,
		).Scan(&eventType, &details, &occurredAt)
	}); err != nil {
		errJSON(w, http.StatusNotFound, "rotation job not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":     jobIDStr,
		"status":     eventType,
		"details":    details,
		"updated_at": occurredAt.Format(time.RFC3339),
	})
}

// ListKeyEvents returns the key event audit trail for the tenant.
func (h *byokHandler) ListKeyEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, err := tenantIDFromRequest(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "tenant_id required")
		return
	}

	type eventRow struct {
		ID          string         `json:"id"`
		EventType   string         `json:"event_type"`
		KeyRef      string         `json:"key_ref"`
		TriggeredBy *string        `json:"triggered_by"`
		OccurredAt  string         `json:"occurred_at"`
		Details     map[string]any `json:"details"`
	}
	var events []eventRow
	err = tenantdb.With(r.Context(), h.db, tenantID, func(tx pgx.Tx) error {
		rows, qErr := tx.Query(r.Context(),
			`SELECT id, event_type, key_ref, triggered_by, occurred_at, details
		   FROM np_encryption_key_events
		  WHERE tenant_id = $1
		  ORDER BY occurred_at DESC LIMIT 100`, tenantID)
		if qErr != nil {
			return qErr
		}
		defer rows.Close()
		for rows.Next() {
			var e eventRow
			var triggeredBy *uuid.UUID
			var occurredAt time.Time
			var id uuid.UUID
			if err := rows.Scan(&id, &e.EventType, &e.KeyRef, &triggeredBy, &occurredAt, &e.Details); err != nil {
				continue
			}
			e.ID = id.String()
			e.OccurredAt = occurredAt.Format(time.RFC3339)
			if triggeredBy != nil {
				s := triggeredBy.String()
				e.TriggeredBy = &s
			}
			events = append(events, e)
		}
		return rows.Err()
	})
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "count": len(events)})
}
