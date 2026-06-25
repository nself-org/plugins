package internal

import (
	"encoding/json"
	"net/http"
	"time"
)

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
// AUTHZ: the body user_id must equal the authenticated principal (CR-C #2).
// Size-cap exception: single-responsibility HTTP route handler — 57L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func (h *Handlers) RegisterIdentity(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req RegisterIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UserID == "" || req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "user_id and device_id are required")
		return
	}
	if req.UserID != p.UserID {
		writeError(w, http.StatusForbidden, "cannot register keys for another user")
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

	ctx := r.Context()
	tx, err := beginScoped(ctx, h.db, p.SourceAccount, p.UserID)
	if err != nil {
		serverError(w, "register: begin", err)
		return
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx,
		`INSERT INTO np_e2ee_identity_keys
		     (source_account_id, user_id, device_id, identity_key_public, registration_id)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (source_account_id, user_id, device_id)
		 DO UPDATE SET identity_key_public = EXCLUDED.identity_key_public,
		               registration_id    = EXCLUDED.registration_id,
		               is_active          = true,
		               last_used_at       = NOW()`,
		p.SourceAccount, req.UserID, req.DeviceID, idKey, req.RegistrationID); err != nil {
		serverError(w, "register: upsert", err)
		return
	}
	h.auditTx(ctx, tx, p.SourceAccount, req.UserID, "identity_registered")
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "register: commit", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

// ============================================================================
// Signed prekey upload (signature-verified)
// ============================================================================

// UploadSignedPreKey verifies the Ed25519 signature against the device identity
// key, then stores the signed prekey and marks any prior one inactive.
// Size-cap exception: single-responsibility HTTP route handler — 75L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func (h *Handlers) UploadSignedPreKey(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req SignedPreKeyUpload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UserID == "" || req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "user_id and device_id are required")
		return
	}
	if req.UserID != p.UserID {
		writeError(w, http.StatusForbidden, "cannot upload prekeys for another user")
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

	ctx := r.Context()
	tx, err := beginScoped(ctx, h.db, p.SourceAccount, p.UserID)
	if err != nil {
		serverError(w, "signed-prekey: begin", err)
		return
	}
	defer tx.Rollback(ctx)

	// Fetch the device identity key (within scope) to verify the signature.
	idKey, err := identityKeyTx(ctx, tx, req.UserID, req.DeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "identity key not registered for device")
		return
	}
	if err := VerifySignedPreKey(idKey, pub, sig); err != nil {
		writeError(w, http.StatusBadRequest, "signed prekey signature verification failed")
		return
	}

	if _, err := tx.Exec(ctx,
		`UPDATE np_e2ee_signed_prekeys SET is_active = false
		  WHERE user_id = $1 AND device_id = $2 AND is_active = true`,
		req.UserID, req.DeviceID); err != nil {
		serverError(w, "signed-prekey: deactivate", err)
		return
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO np_e2ee_signed_prekeys
		     (source_account_id, user_id, device_id, key_id, public_key, signature, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, true)
		 ON CONFLICT (source_account_id, user_id, device_id, key_id)
		 DO UPDATE SET public_key = EXCLUDED.public_key,
		               signature  = EXCLUDED.signature,
		               is_active  = true`,
		p.SourceAccount, req.UserID, req.DeviceID, req.KeyID, pub, sig); err != nil {
		serverError(w, "signed-prekey: insert", err)
		return
	}
	h.auditTx(ctx, tx, p.SourceAccount, req.UserID, "signed_prekey_uploaded")
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "signed-prekey: commit", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "stored", "key_id": req.KeyID})
}

// ============================================================================
// One-time + Kyber prekey batch upload
// ============================================================================

// UploadOneTimePreKeys batch-stores classic + Kyber one-time PUBLIC prekeys.
// Each Kyber prekey is structurally validated AND signature-verified against
// the supplied identity key before insertion.
// Size-cap exception: single-responsibility HTTP route handler — 92L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func (h *Handlers) UploadOneTimePreKeys(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req UploadOneTimePreKeysRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UserID == "" || req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "user_id and device_id are required")
		return
	}
	if req.UserID != p.UserID {
		writeError(w, http.StatusForbidden, "cannot upload prekeys for another user")
		return
	}
	if len(req.OneTimeKeys) > h.cfg.MaxOneTimePreKeys || len(req.KyberPreKeys) > h.cfg.MaxKyberPreKeys {
		writeError(w, http.StatusBadRequest, "too many prekeys in one batch")
		return
	}

	ctx := r.Context()
	tx, err := beginScoped(ctx, h.db, p.SourceAccount, p.UserID)
	if err != nil {
		serverError(w, "one-time: begin", err)
		return
	}
	defer tx.Rollback(ctx)

	// Resolve the STORED identity key (within scope) for Kyber-signature
	// verification. Never trusts a request-body identity key.
	idKey, err := identityKeyTx(ctx, tx, req.UserID, req.DeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "identity key not registered for device")
		return
	}

	for _, k := range req.OneTimeKeys {
		pub, derr := decodeB64(k.PublicKey)
		if derr != nil {
			writeError(w, http.StatusBadRequest, "one_time public_key must be base64")
			return
		}
		if _, derr = tx.Exec(ctx,
			`INSERT INTO np_e2ee_one_time_prekeys (source_account_id, user_id, device_id, key_id, public_key)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (source_account_id, user_id, device_id, key_id) DO NOTHING`,
			p.SourceAccount, req.UserID, req.DeviceID, k.KeyID, pub); derr != nil {
			serverError(w, "one-time: insert otpk", derr)
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
			writeError(w, http.StatusBadRequest, "kyber prekey signature verification failed")
			return
		}
		if _, derr = tx.Exec(ctx,
			`INSERT INTO np_e2ee_kyber_prekeys (source_account_id, user_id, device_id, key_id, kyber_public_key, kyber_signature)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (source_account_id, user_id, device_id, key_id) DO NOTHING`,
			p.SourceAccount, req.UserID, req.DeviceID, k.KeyID, pub, sig); derr != nil {
			serverError(w, "one-time: insert kyber", derr)
			return
		}
	}

	h.auditTx(ctx, tx, p.SourceAccount, req.UserID, "prekeys_replenished")
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "one-time: commit", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "stored",
		"one_time_stored": len(req.OneTimeKeys),
		"kyber_stored":    len(req.KyberPreKeys),
	})
}
