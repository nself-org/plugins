package internal

import (
	"encoding/base64"
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	pgx "github.com/jackc/pgx/v5"
)


// ============================================================================
// Prekey bundle (atomic consumption)
// ============================================================================

// GetPreKeyBundle returns a prekey bundle for an X3DH+PQ initiator, consuming
// one classic one-time prekey and one Kyber prekey ATOMICALLY. If either is
// exhausted, the bundle is still returned (signed prekey only) — graceful
// degradation per the Signal protocol.
// Size-cap exception: single-responsibility HTTP route handler — 133L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func (h *Handlers) GetPreKeyBundle(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	userID := chi.URLParam(r, "userId")
	deviceID := r.URL.Query().Get("device_id")
	// CR-C #3: the requester is the AUTHENTICATED principal, never a query param.
	// This prevents forging the audit trail and ties per-requester rate limits
	// to a real identity.
	requestedBy := p.UserID
	if userID == "" || deviceID == "" {
		writeError(w, http.StatusBadRequest, "userId path param and device_id query are required")
		return
	}

	ctx := r.Context()
	tx, err := beginScoped(ctx, h.db, p.SourceAccount, p.UserID)
	if err != nil {
		serverError(w, "bundle: begin", err)
		return
	}
	defer tx.Rollback(ctx)

	var resp PreKeyBundleResponse
	resp.UserID = userID
	resp.DeviceID = deviceID

	// Identity + registration.
	var idKey []byte
	if err := tx.QueryRow(ctx,
		`SELECT identity_key_public, registration_id
		   FROM np_e2ee_identity_keys
		  WHERE user_id = $1 AND device_id = $2 AND is_active = true`,
		userID, deviceID).Scan(&idKey, &resp.RegistrationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no active identity for user/device")
			return
		}
		serverError(w, "bundle: identity", err)
		return
	}
	resp.IdentityKeyPublic = base64.StdEncoding.EncodeToString(idKey)

	// Active signed prekey (required).
	var spkPub, spkSig []byte
	if err := tx.QueryRow(ctx,
		`SELECT key_id, public_key, signature
		   FROM np_e2ee_signed_prekeys
		  WHERE user_id = $1 AND device_id = $2 AND is_active = true
		  ORDER BY created_at DESC LIMIT 1`,
		userID, deviceID).Scan(&resp.SignedPreKeyID, &spkPub, &spkSig); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "no active signed prekey for user/device")
			return
		}
		serverError(w, "bundle: signed-prekey", err)
		return
	}
	resp.SignedPreKeyPublic = base64.StdEncoding.EncodeToString(spkPub)
	resp.SignedPreKeySignature = base64.StdEncoding.EncodeToString(spkSig)

	// Atomically consume one classic one-time prekey (if any remain).
	// The CTE selects the lowest-key_id unconsumed row FOR UPDATE SKIP LOCKED
	// and flips is_consumed in a single statement — no double-issue window.
	var otpkID int
	var otpkPub []byte
	otpkErr := tx.QueryRow(ctx,
		`UPDATE np_e2ee_one_time_prekeys
		    SET is_consumed = true, consumed_at = NOW(), consumed_by = $3
		  WHERE id = (
		        SELECT id FROM np_e2ee_one_time_prekeys
		         WHERE user_id = $1 AND device_id = $2 AND is_consumed = false
		         ORDER BY key_id ASC
		         FOR UPDATE SKIP LOCKED
		         LIMIT 1)
		  RETURNING key_id, public_key`,
		userID, deviceID, nullable(requestedBy)).Scan(&otpkID, &otpkPub)
	if otpkErr == nil {
		pub := base64.StdEncoding.EncodeToString(otpkPub)
		resp.OneTimePreKeyID = &otpkID
		resp.OneTimePreKeyPublic = &pub
	} else if !errors.Is(otpkErr, pgx.ErrNoRows) {
		serverError(w, "bundle: consume otpk", otpkErr)
		return
	}

	// Atomically consume one Kyber prekey (if any remain).
	var kID int
	var kPub, kSig []byte
	kErr := tx.QueryRow(ctx,
		`UPDATE np_e2ee_kyber_prekeys
		    SET is_consumed = true, consumed_at = NOW(), consumed_by = $3
		  WHERE id = (
		        SELECT id FROM np_e2ee_kyber_prekeys
		         WHERE user_id = $1 AND device_id = $2 AND is_consumed = false
		         ORDER BY key_id ASC
		         FOR UPDATE SKIP LOCKED
		         LIMIT 1)
		  RETURNING key_id, kyber_public_key, kyber_signature`,
		userID, deviceID, nullable(requestedBy)).Scan(&kID, &kPub, &kSig)
	if kErr == nil {
		pub := base64.StdEncoding.EncodeToString(kPub)
		sig := base64.StdEncoding.EncodeToString(kSig)
		resp.KyberPreKeyID = &kID
		resp.KyberPreKeyPublic = &pub
		resp.KyberPreKeySignature = &sig
	} else if !errors.Is(kErr, pgx.ErrNoRows) {
		serverError(w, "bundle: consume kyber", kErr)
		return
	}

	// Record which bundle was served (audit of consumption). requested_by is the
	// authenticated principal, which the RLS insert policy requires to equal
	// current_setting('app.current_user_id').
	if _, err := tx.Exec(ctx,
		`INSERT INTO np_e2ee_prekey_bundles_served
		     (source_account_id, target_user_id, target_device_id, requested_by,
		      signed_prekey_id, one_time_prekey_id, kyber_prekey_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		p.SourceAccount, userID, deviceID, requestedBy,
		resp.SignedPreKeyID, resp.OneTimePreKeyID, resp.KyberPreKeyID); err != nil {
		serverError(w, "bundle: audit insert", err)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		serverError(w, "bundle: commit", err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// CheckReplenish reports remaining unconsumed prekey counts for a device.
