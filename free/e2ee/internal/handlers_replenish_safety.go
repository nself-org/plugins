package internal

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
)

func (h *Handlers) CheckReplenish(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	userID := chi.URLParam(r, "userId")
	deviceID := r.URL.Query().Get("device_id")
	if userID == "" || deviceID == "" {
		writeError(w, http.StatusBadRequest, "userId and device_id are required")
		return
	}
	ctx := r.Context()
	tx, err := beginScoped(ctx, h.db, p.SourceAccount, p.UserID)
	if err != nil {
		serverError(w, "replenish: begin", err)
		return
	}
	defer tx.Rollback(ctx)

	var st ReplenishStatus
	st.UserID = userID
	st.DeviceID = deviceID
	if err := tx.QueryRow(ctx,
		`SELECT
		   (SELECT count(*) FROM np_e2ee_one_time_prekeys
		     WHERE user_id=$1 AND device_id=$2 AND is_consumed=false),
		   (SELECT count(*) FROM np_e2ee_kyber_prekeys
		     WHERE user_id=$1 AND device_id=$2 AND is_consumed=false)`,
		userID, deviceID).Scan(&st.OneTimeRemaining, &st.KyberRemaining); err != nil {
		serverError(w, "replenish: count", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "replenish: commit", err)
		return
	}
	st.NeedsReplenish = st.OneTimeRemaining < 10 || st.KyberRemaining < 10
	writeJSON(w, http.StatusOK, st)
}

// ============================================================================
// Safety numbers / verification state
// ============================================================================

// PostSafetyNumber upserts a computed safety number + verification flag.
// AUTHZ: the body user_id must equal the authenticated principal.
// Size-cap exception: single-responsibility HTTP route handler — 63L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func (h *Handlers) PostSafetyNumber(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req SafetyNumberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UserID == "" || req.PeerUserID == "" || req.SafetyNumber == "" {
		writeError(w, http.StatusBadRequest, "user_id, peer_user_id, safety_number required")
		return
	}
	if req.UserID != p.UserID {
		writeError(w, http.StatusForbidden, "cannot post safety number for another user")
		return
	}
	ctx := r.Context()
	tx, err := beginScoped(ctx, h.db, p.SourceAccount, p.UserID)
	if err != nil {
		serverError(w, "safety: begin", err)
		return
	}
	defer tx.Rollback(ctx)

	var verifiedAt any
	if req.IsVerified {
		verifiedAt = time.Now().UTC()
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO np_e2ee_safety_numbers
		     (source_account_id, user_id, peer_user_id, safety_number, is_verified, verified_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (source_account_id, user_id, peer_user_id)
		 DO UPDATE SET safety_number = EXCLUDED.safety_number,
		               is_verified   = EXCLUDED.is_verified,
		               verified_at   = EXCLUDED.verified_at`,
		p.SourceAccount, req.UserID, req.PeerUserID, req.SafetyNumber, req.IsVerified, verifiedAt); err != nil {
		serverError(w, "safety: upsert number", err)
		return
	}
	state := "unverified"
	if req.IsVerified {
		state = "verified"
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO np_e2ee_verification_states (source_account_id, user_id, peer_user_id, state)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (source_account_id, user_id, peer_user_id)
		 DO UPDATE SET state = EXCLUDED.state, updated_at = NOW()`,
		p.SourceAccount, req.UserID, req.PeerUserID, state); err != nil {
		serverError(w, "safety: upsert state", err)
		return
	}
	h.auditTx(ctx, tx, p.SourceAccount, req.UserID, "safety_number_posted")
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "safety: commit", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stored", "state": state})
}

// GetVerificationState returns the verification state for a (user, peer) pair.
// AUTHZ: a caller may only read their OWN verification state.
