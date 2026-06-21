package internal

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"log"
	"net/http"
	pgx "github.com/jackc/pgx/v5"
)

func (h *Handlers) GetVerificationState(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	userID := chi.URLParam(r, "userId")
	peerID := chi.URLParam(r, "peerId")
	if userID == "" || peerID == "" {
		writeError(w, http.StatusBadRequest, "userId and peerId are required")
		return
	}
	if userID != p.UserID {
		writeError(w, http.StatusForbidden, "cannot read another user's verification state")
		return
	}
	ctx := r.Context()
	tx, err := beginScoped(ctx, h.db, p.SourceAccount, p.UserID)
	if err != nil {
		serverError(w, "verification: begin", err)
		return
	}
	defer tx.Rollback(ctx)

	var vs VerificationState
	vs.UserID = userID
	vs.PeerUserID = peerID
	err = tx.QueryRow(ctx,
		`SELECT state, updated_at FROM np_e2ee_verification_states
		  WHERE user_id = $1 AND peer_user_id = $2`,
		userID, peerID).Scan(&vs.State, &vs.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Commit(ctx)
		vs.State = "unverified"
		writeJSON(w, http.StatusOK, vs)
		return
	}
	if err != nil {
		serverError(w, "verification: query", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "verification: commit", err)
		return
	}
	writeJSON(w, http.StatusOK, vs)
}

// ListAudit returns recent append-only audit entries for a user.
// AUTHZ: a caller may only read their OWN audit log.
func (h *Handlers) ListAudit(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "userId is required")
		return
	}
	if userID != p.UserID {
		writeError(w, http.StatusForbidden, "cannot read another user's audit log")
		return
	}
	ctx := r.Context()
	tx, err := beginScoped(ctx, h.db, p.SourceAccount, p.UserID)
	if err != nil {
		serverError(w, "audit: begin", err)
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT id, user_id, event_type, created_at
		   FROM np_e2ee_audit_log
		  WHERE user_id = $1
		  ORDER BY created_at DESC LIMIT 100`, userID)
	if err != nil {
		serverError(w, "audit: query", err)
		return
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.EventType, &e.CreatedAt); err != nil {
			serverError(w, "audit: scan", err)
			return
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		serverError(w, "audit: rows", err)
		return
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "audit: commit", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ============================================================================
// Internal helpers
// ============================================================================

// identityKeyTx loads the active PUBLIC identity key for a user/device within
// the supplied (GUC-scoped) transaction so RLS public-read applies.
func identityKeyTx(ctx context.Context, tx pgx.Tx, userID, deviceID string) ([]byte, error) {
	var idKey []byte
	err := tx.QueryRow(ctx,
		`SELECT identity_key_public FROM np_e2ee_identity_keys
		  WHERE user_id = $1 AND device_id = $2 AND is_active = true`,
		userID, deviceID).Scan(&idKey)
	if err != nil {
		return nil, err
	}
	return idKey, nil
}

// auditTx writes a best-effort append-only audit row inside the active tx (so
// the GUCs the audit-insert RLS policy reads are set). Failures are logged but
// do not abort the surrounding transaction.
func (h *Handlers) auditTx(ctx context.Context, tx pgx.Tx, sourceAccount, userID, eventType string) {
	if _, err := tx.Exec(ctx,
		`INSERT INTO np_e2ee_audit_log (source_account_id, user_id, event_type)
		 VALUES ($1, $2, $3)`,
		sourceAccount, userID, eventType); err != nil {
		log.Printf("e2ee: audit insert (%s): %v", eventType, err)
	}
}

// nullable returns nil for an empty string so it lands as SQL NULL.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
