package internal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handlers serves the e2ee key-directory REST API.
//
// Purpose:   HTTP layer over the np_e2ee_* tables. Stores/serves PUBLIC keys.
// Inputs:    JSON requests (see models.go); pgx pool.
// Outputs:   JSON responses; rows in np_e2ee_* tables.
// Constraints (CR-C critical):
//   - No endpoint accepts, returns, or persists private key material.
//   - One-time + Kyber prekeys are consumed atomically (UPDATE ... RETURNING in
//     a transaction) so a prekey can be handed out at most once (no replay window).
//   - Signed prekeys + Kyber prekeys are signature-verified before storage.
type Handlers struct {
	db  *pgxpool.Pool
	cfg *Config
}

// NewHandlers builds a Handlers with config from the environment.
func NewHandlers(db *pgxpool.Pool) *Handlers {
	return NewHandlersFromConfig(db, LoadConfig())
}

// NewHandlersFromConfig builds a Handlers with an explicit config (used in tests).
func NewHandlersFromConfig(db *pgxpool.Pool, cfg *Config) *Handlers {
	if cfg == nil {
		cfg = LoadConfig()
	}
	return &Handlers{db: db, cfg: cfg}
}

func bg() context.Context { return context.Background() }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeB64 decodes a base64 (std) string, returning a 400-friendly error.
func decodeB64(s string) ([]byte, error) {
	if s == "" {
		return nil, errors.New("empty value")
	}
	return base64.StdEncoding.DecodeString(s)
}

// ============================================================================
// Health
// ============================================================================

// Health returns 200 OK with plugin metadata.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"plugin":    "e2ee",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "1.0.0",
	})
}

// Ready checks database connectivity.
func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := h.db.Ping(bg()); err != nil {
		dbStatus = "error"
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"database":  dbStatus,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ============================================================================
// Identity registration
// ============================================================================

// RegisterIdentity stores (or upserts) a device's PUBLIC identity key.
func (h *Handlers) RegisterIdentity(w http.ResponseWriter, r *http.Request) {
	var req RegisterIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UserID == "" || req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "user_id and device_id are required")
		return
	}
	idKey, err := decodeB64(req.IdentityKeyPublic)
	if err != nil {
		writeError(w, http.StatusBadRequest, "identity_key_public must be base64")
		return
	}
	if len(idKey) != Ed25519PublicKeySize {
		writeError(w, http.StatusBadRequest, ErrInvalidIdentityKey.Error())
		return
	}

	_, err = h.db.Exec(bg(),
		`INSERT INTO np_e2ee_identity_keys
		     (user_id, device_id, identity_key_public, registration_id)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (source_account_id, user_id, device_id)
		 DO UPDATE SET identity_key_public = EXCLUDED.identity_key_public,
		               registration_id    = EXCLUDED.registration_id,
		               is_active          = true,
		               last_used_at       = NOW()`,
		req.UserID, req.DeviceID, idKey, req.RegistrationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(req.UserID, "identity_registered")
	writeJSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

// ============================================================================
// Signed prekey upload (signature-verified)
// ============================================================================

// UploadSignedPreKey verifies the Ed25519 signature against the device identity
// key, then stores the signed prekey and marks any prior one inactive.
func (h *Handlers) UploadSignedPreKey(w http.ResponseWriter, r *http.Request) {
	var req SignedPreKeyUpload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UserID == "" || req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "user_id and device_id are required")
		return
	}
	pub, err := decodeB64(req.PublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "public_key must be base64")
		return
	}
	sig, err := decodeB64(req.Signature)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signature must be base64")
		return
	}

	// Fetch the device identity key to verify the signature.
	idKey, err := h.identityKey(req.UserID, req.DeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "identity key not registered for device")
		return
	}
	if err := VerifySignedPreKey(idKey, pub, sig); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := bg()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE np_e2ee_signed_prekeys SET is_active = false
		  WHERE user_id = $1 AND device_id = $2 AND is_active = true`,
		req.UserID, req.DeviceID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO np_e2ee_signed_prekeys
		     (user_id, device_id, key_id, public_key, signature, is_active)
		 VALUES ($1, $2, $3, $4, $5, true)
		 ON CONFLICT (source_account_id, user_id, device_id, key_id)
		 DO UPDATE SET public_key = EXCLUDED.public_key,
		               signature  = EXCLUDED.signature,
		               is_active  = true`,
		req.UserID, req.DeviceID, req.KeyID, pub, sig); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(req.UserID, "signed_prekey_uploaded")
	writeJSON(w, http.StatusOK, map[string]any{"status": "stored", "key_id": req.KeyID})
}

// ============================================================================
// One-time + Kyber prekey batch upload
// ============================================================================

// UploadOneTimePreKeys batch-stores classic + Kyber one-time PUBLIC prekeys.
// Each Kyber prekey is structurally validated AND signature-verified against
// the supplied identity key before insertion.
func (h *Handlers) UploadOneTimePreKeys(w http.ResponseWriter, r *http.Request) {
	var req UploadOneTimePreKeysRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UserID == "" || req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "user_id and device_id are required")
		return
	}
	if len(req.OneTimeKeys) > h.cfg.MaxOneTimePreKeys || len(req.KyberPreKeys) > h.cfg.MaxKyberPreKeys {
		writeError(w, http.StatusBadRequest, "too many prekeys in one batch")
		return
	}

	// Resolve identity key for Kyber-signature verification: prefer the stored
	// one; fall back to the supplied one only if it matches what is on file.
	idKey, err := h.identityKey(req.UserID, req.DeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "identity key not registered for device")
		return
	}

	ctx := bg()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(ctx)

	for _, k := range req.OneTimeKeys {
		pub, derr := decodeB64(k.PublicKey)
		if derr != nil {
			writeError(w, http.StatusBadRequest, "one_time public_key must be base64")
			return
		}
		if _, derr = tx.Exec(ctx,
			`INSERT INTO np_e2ee_one_time_prekeys (user_id, device_id, key_id, public_key)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (source_account_id, user_id, device_id, key_id) DO NOTHING`,
			req.UserID, req.DeviceID, k.KeyID, pub); derr != nil {
			writeError(w, http.StatusInternalServerError, derr.Error())
			return
		}
	}

	for _, k := range req.KyberPreKeys {
		pub, derr := decodeB64(k.PublicKey)
		if derr != nil {
			writeError(w, http.StatusBadRequest, "kyber public_key must be base64")
			return
		}
		sig, derr := decodeB64(k.Signature)
		if derr != nil {
			writeError(w, http.StatusBadRequest, "kyber signature must be base64")
			return
		}
		if verr := VerifyKyberPreKey(idKey, pub, sig); verr != nil {
			writeError(w, http.StatusBadRequest, verr.Error())
			return
		}
		if _, derr = tx.Exec(ctx,
			`INSERT INTO np_e2ee_kyber_prekeys (user_id, device_id, key_id, kyber_public_key, kyber_signature)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (source_account_id, user_id, device_id, key_id) DO NOTHING`,
			req.UserID, req.DeviceID, k.KeyID, pub, sig); derr != nil {
			writeError(w, http.StatusInternalServerError, derr.Error())
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(req.UserID, "prekeys_replenished")
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "stored",
		"one_time_stored": len(req.OneTimeKeys),
		"kyber_stored":    len(req.KyberPreKeys),
	})
}

// ============================================================================
// Prekey bundle (atomic consumption)
// ============================================================================

// GetPreKeyBundle returns a prekey bundle for an X3DH+PQ initiator, consuming
// one classic one-time prekey and one Kyber prekey ATOMICALLY. If either is
// exhausted, the bundle is still returned (signed prekey only) — graceful
// degradation per the Signal protocol.
func (h *Handlers) GetPreKeyBundle(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	deviceID := r.URL.Query().Get("device_id")
	requestedBy := r.URL.Query().Get("requested_by")
	if userID == "" || deviceID == "" {
		writeError(w, http.StatusBadRequest, "userId path param and device_id query are required")
		return
	}

	ctx := bg()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, otpkErr.Error())
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
		writeError(w, http.StatusInternalServerError, kErr.Error())
		return
	}

	// Record which bundle was served (audit of consumption).
	if _, err := tx.Exec(ctx,
		`INSERT INTO np_e2ee_prekey_bundles_served
		     (target_user_id, target_device_id, requested_by,
		      signed_prekey_id, one_time_prekey_id, kyber_prekey_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, deviceID, orUnknown(requestedBy),
		resp.SignedPreKeyID, resp.OneTimePreKeyID, resp.KyberPreKeyID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// CheckReplenish reports remaining unconsumed prekey counts for a device.
func (h *Handlers) CheckReplenish(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	deviceID := r.URL.Query().Get("device_id")
	if userID == "" || deviceID == "" {
		writeError(w, http.StatusBadRequest, "userId and device_id are required")
		return
	}
	var st ReplenishStatus
	st.UserID = userID
	st.DeviceID = deviceID
	_ = h.db.QueryRow(bg(),
		`SELECT
		   (SELECT count(*) FROM np_e2ee_one_time_prekeys
		     WHERE user_id=$1 AND device_id=$2 AND is_consumed=false),
		   (SELECT count(*) FROM np_e2ee_kyber_prekeys
		     WHERE user_id=$1 AND device_id=$2 AND is_consumed=false)`,
		userID, deviceID).Scan(&st.OneTimeRemaining, &st.KyberRemaining)
	st.NeedsReplenish = st.OneTimeRemaining < 10 || st.KyberRemaining < 10
	writeJSON(w, http.StatusOK, st)
}

// ============================================================================
// Safety numbers / verification state
// ============================================================================

// PostSafetyNumber upserts a computed safety number + verification flag.
func (h *Handlers) PostSafetyNumber(w http.ResponseWriter, r *http.Request) {
	var req SafetyNumberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UserID == "" || req.PeerUserID == "" || req.SafetyNumber == "" {
		writeError(w, http.StatusBadRequest, "user_id, peer_user_id, safety_number required")
		return
	}
	ctx := bg()
	var verifiedAt any
	if req.IsVerified {
		verifiedAt = time.Now().UTC()
	}
	if _, err := h.db.Exec(ctx,
		`INSERT INTO np_e2ee_safety_numbers
		     (user_id, peer_user_id, safety_number, is_verified, verified_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (source_account_id, user_id, peer_user_id)
		 DO UPDATE SET safety_number = EXCLUDED.safety_number,
		               is_verified   = EXCLUDED.is_verified,
		               verified_at   = EXCLUDED.verified_at`,
		req.UserID, req.PeerUserID, req.SafetyNumber, req.IsVerified, verifiedAt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	state := "unverified"
	if req.IsVerified {
		state = "verified"
	}
	if _, err := h.db.Exec(ctx,
		`INSERT INTO np_e2ee_verification_states (user_id, peer_user_id, state)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (source_account_id, user_id, peer_user_id)
		 DO UPDATE SET state = EXCLUDED.state, updated_at = NOW()`,
		req.UserID, req.PeerUserID, state); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(req.UserID, "safety_number_posted")
	writeJSON(w, http.StatusOK, map[string]string{"status": "stored", "state": state})
}

// GetVerificationState returns the verification state for a (user, peer) pair.
func (h *Handlers) GetVerificationState(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	peerID := chi.URLParam(r, "peerId")
	if userID == "" || peerID == "" {
		writeError(w, http.StatusBadRequest, "userId and peerId are required")
		return
	}
	var vs VerificationState
	vs.UserID = userID
	vs.PeerUserID = peerID
	err := h.db.QueryRow(bg(),
		`SELECT state, updated_at FROM np_e2ee_verification_states
		  WHERE user_id = $1 AND peer_user_id = $2`,
		userID, peerID).Scan(&vs.State, &vs.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		vs.State = "unverified"
		writeJSON(w, http.StatusOK, vs)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, vs)
}

// ListAudit returns recent append-only audit entries for a user.
func (h *Handlers) ListAudit(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "userId is required")
		return
	}
	rows, err := h.db.Query(bg(),
		`SELECT id, user_id, event_type, created_at
		   FROM np_e2ee_audit_log
		  WHERE user_id = $1
		  ORDER BY created_at DESC LIMIT 100`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.EventType, &e.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
}

// ============================================================================
// Internal helpers
// ============================================================================

// identityKey loads the active PUBLIC identity key for a user/device.
func (h *Handlers) identityKey(userID, deviceID string) ([]byte, error) {
	var idKey []byte
	err := h.db.QueryRow(bg(),
		`SELECT identity_key_public FROM np_e2ee_identity_keys
		  WHERE user_id = $1 AND device_id = $2 AND is_active = true`,
		userID, deviceID).Scan(&idKey)
	if err != nil {
		return nil, err
	}
	return idKey, nil
}

// audit writes a best-effort append-only audit row. Failures are swallowed so a
// logging error never blocks the primary operation.
func (h *Handlers) audit(userID, eventType string) {
	_, _ = h.db.Exec(bg(),
		`INSERT INTO np_e2ee_audit_log (user_id, event_type) VALUES ($1, $2)`,
		userID, eventType)
}

// nullable returns nil for an empty string so it lands as SQL NULL.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// orUnknown returns "unknown" for an empty requester (NOT NULL column).
func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
