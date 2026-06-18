package internal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
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

// serverError logs the full error server-side and returns a GENERIC 500 to the
// client (CR-C MED fix: never leak err.Error() — internal detail to clients).
func serverError(w http.ResponseWriter, where string, err error) {
	log.Printf("e2ee: %s: %v", where, err)
	writeError(w, http.StatusInternalServerError, "internal error")
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
// AUTHZ: the body user_id must equal the authenticated principal (CR-C #2).
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

// ============================================================================
// Prekey bundle (atomic consumption)
// ============================================================================

// GetPreKeyBundle returns a prekey bundle for an X3DH+PQ initiator, consuming
// one classic one-time prekey and one Kyber prekey ATOMICALLY. If either is
// exhausted, the bundle is still returned (signed prekey only) — graceful
// degradation per the Signal protocol.
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
