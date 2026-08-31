package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nself-org/nself-vault/internal/auth"
	"github.com/nself-org/nself-vault/internal/store"
)

// Handler holds dependencies for all HTTP handlers.
type Handler struct {
	db   *store.DB
	auth *auth.Extractor
}

// New returns a Handler with the given store and JWT extractor.
func New(db *store.DB, extractor *auth.Extractor) *Handler {
	return &Handler{db: db, auth: extractor}
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handler) mustAuth(w http.ResponseWriter, r *http.Request) (auth.UserInfo, bool) {
	info, err := h.auth.FromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return auth.UserInfo{}, false
	}
	return info, true
}

func b64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// auditAsync fires an audit log write in a goroutine so it never blocks the response.
// Errors are logged but do not fail the request (best-effort audit).
func (h *Handler) auditAsync(userID, tenantID, action string, recordID, deviceID *string, meta map[string]interface{}) {
	go func() {
		// Request context may already be cancelled — use a fresh background context.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.db.WriteAudit(ctx, userID, tenantID, action, recordID, deviceID, meta); err != nil {
			log.Printf("vault: audit write failed (action=%s user=%s): %v", action, userID, err)
		}
	}()
}

// ---------- /vault/v1/devices ----------

// PostDevice handles POST /vault/v1/devices — register a new device.
func (h *Handler) PostDevice(w http.ResponseWriter, r *http.Request) {
	info, ok := h.mustAuth(w, r)
	if !ok {
		return
	}

	var body struct {
		DevicePubkey string `json:"device_pubkey"` // base64-encoded public key bytes
		DeviceLabel  string `json:"device_label"`
		Platform     string `json:"platform"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.DevicePubkey == "" {
		writeErr(w, http.StatusBadRequest, "device_pubkey required")
		return
	}
	pubkey, err := b64Decode(body.DevicePubkey)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "device_pubkey must be base64-encoded")
		return
	}

	id, err := h.db.CreateDevice(r.Context(), info.UserID, info.TenantID, pubkey, body.DeviceLabel, body.Platform)
	if err != nil {
		log.Printf("vault: create device error (user=%s): %v", info.UserID, err)
		writeErr(w, http.StatusInternalServerError, "failed to register device")
		return
	}

	h.auditAsync(info.UserID, info.TenantID, "device.register", nil, &id, nil)
	writeJSON(w, http.StatusCreated, map[string]string{"device_id": id})
}

// GetDevices handles GET /vault/v1/devices — list user's non-revoked devices.
func (h *Handler) GetDevices(w http.ResponseWriter, r *http.Request) {
	info, ok := h.mustAuth(w, r)
	if !ok {
		return
	}

	devices, err := h.db.ListDevices(r.Context(), info.UserID)
	if err != nil {
		log.Printf("vault: list devices error (user=%s): %v", info.UserID, err)
		writeErr(w, http.StatusInternalServerError, "failed to list devices")
		return
	}
	if devices == nil {
		devices = []store.Device{}
	}

	// Strip raw pubkey bytes from list response — return only metadata.
	type deviceItem struct {
		ID          string `json:"id"`
		DeviceLabel string `json:"device_label"`
		Platform    string `json:"platform"`
		LastSeenAt  string `json:"last_seen_at"`
	}
	out := make([]deviceItem, len(devices))
	for i, d := range devices {
		out[i] = deviceItem{
			ID:          d.ID,
			DeviceLabel: d.DeviceLabel,
			Platform:    d.Platform,
			LastSeenAt:  d.LastSeenAt.Format(time.RFC3339),
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": out})
}

// DeleteDevice handles DELETE /vault/v1/devices/{id} — revoke a device.
func (h *Handler) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	info, ok := h.mustAuth(w, r)
	if !ok {
		return
	}
	deviceID := chi.URLParam(r, "id")
	if deviceID == "" {
		writeErr(w, http.StatusBadRequest, "device id required")
		return
	}

	if err := h.db.RevokeDevice(r.Context(), deviceID, info.UserID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "device not found")
			return
		}
		log.Printf("vault: revoke device error (device=%s user=%s): %v", deviceID, info.UserID, err)
		writeErr(w, http.StatusInternalServerError, "failed to revoke device")
		return
	}

	h.auditAsync(info.UserID, info.TenantID, "device.revoke", nil, &deviceID, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// ---------- /vault/v1/records ----------

// PostRecord handles POST /vault/v1/records — create record + initial envelopes.
func (h *Handler) PostRecord(w http.ResponseWriter, r *http.Request) {
	info, ok := h.mustAuth(w, r)
	if !ok {
		return
	}

	var body struct {
		CredentialKind string `json:"credential_kind"`
		Label          string `json:"label"`
		// Initial envelope for the creating device
		DeviceID           string `json:"device_id"`
		EnvelopeCiphertext string `json:"envelope_ciphertext"` // base64
		EnvelopeNonce      string `json:"envelope_nonce"`      // base64
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.CredentialKind == "" || body.Label == "" {
		writeErr(w, http.StatusBadRequest, "credential_kind and label required")
		return
	}
	if body.DeviceID == "" || body.EnvelopeCiphertext == "" || body.EnvelopeNonce == "" {
		writeErr(w, http.StatusBadRequest, "device_id, envelope_ciphertext, envelope_nonce required")
		return
	}

	ciphertext, err := b64Decode(body.EnvelopeCiphertext)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "envelope_ciphertext must be base64-encoded")
		return
	}
	nonce, err := b64Decode(body.EnvelopeNonce)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "envelope_nonce must be base64-encoded")
		return
	}

	// V05-F4: verify the initial device belongs to the authenticated user
	// before creating the record + envelope pair.
	if err := h.db.DeviceBelongsToUser(r.Context(), body.DeviceID, info.UserID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "device not found")
			return
		}
		log.Printf("vault: device ownership check error (device=%s user=%s): %v", body.DeviceID, info.UserID, err)
		writeErr(w, http.StatusInternalServerError, "failed to verify device ownership")
		return
	}

	recordID, err := h.db.CreateRecord(r.Context(), info.UserID, info.TenantID, body.CredentialKind, body.Label)
	if err != nil {
		log.Printf("vault: create record error (user=%s): %v", info.UserID, err)
		writeErr(w, http.StatusInternalServerError, "failed to create record")
		return
	}

	envelopeID, err := h.db.CreateEnvelope(r.Context(), recordID, body.DeviceID, ciphertext, nonce)
	if err != nil {
		log.Printf("vault: create initial envelope error (record=%s device=%s): %v", recordID, body.DeviceID, err)
		writeErr(w, http.StatusInternalServerError, "failed to create initial envelope")
		return
	}
	_ = envelopeID

	h.auditAsync(info.UserID, info.TenantID, "record.create", &recordID, &body.DeviceID, map[string]interface{}{
		"credential_kind": body.CredentialKind,
	})
	writeJSON(w, http.StatusCreated, map[string]string{"record_id": recordID})
}

// GetRecords handles GET /vault/v1/records?since=<rfc3339> — list records for this user.
func (h *Handler) GetRecords(w http.ResponseWriter, r *http.Request) {
	info, ok := h.mustAuth(w, r)
	if !ok {
		return
	}

	var since *time.Time
	if s := r.URL.Query().Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "since must be RFC3339 timestamp")
			return
		}
		since = &t
	}

	records, err := h.db.ListRecords(r.Context(), info.UserID, since)
	if err != nil {
		log.Printf("vault: list records error (user=%s): %v", info.UserID, err)
		writeErr(w, http.StatusInternalServerError, "failed to list records")
		return
	}
	if records == nil {
		records = []store.Record{}
	}

	type recordItem struct {
		ID             string `json:"id"`
		CredentialKind string `json:"credential_kind"`
		Label          string `json:"label"`
		CreatedAt      string `json:"created_at"`
		UpdatedAt      string `json:"updated_at"`
	}
	out := make([]recordItem, len(records))
	for i, rec := range records {
		out[i] = recordItem{
			ID:             rec.ID,
			CredentialKind: rec.CredentialKind,
			Label:          rec.Label,
			CreatedAt:      rec.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      rec.UpdatedAt.Format(time.RFC3339),
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"records": out})
}

// GetEnvelope handles GET /vault/v1/records/{id}/envelope — fetch this device's envelope.
func (h *Handler) GetEnvelope(w http.ResponseWriter, r *http.Request) {
	info, ok := h.mustAuth(w, r)
	if !ok {
		return
	}
	recordID := chi.URLParam(r, "id")
	deviceID := r.URL.Query().Get("device_id")
	if deviceID == "" {
		writeErr(w, http.StatusBadRequest, "device_id query param required")
		return
	}

	// V05-F4: enforce that the device belongs to the authenticated user.
	// Without this check, any user who knows another user's device UUID could
	// fetch their envelope. Return 404 (not 403) to avoid leaking existence.
	if err := h.db.DeviceBelongsToUser(r.Context(), deviceID, info.UserID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "envelope not found")
			return
		}
		log.Printf("vault: device ownership check error (device=%s user=%s): %v", deviceID, info.UserID, err)
		writeErr(w, http.StatusInternalServerError, "failed to verify device ownership")
		return
	}
	// Defense-in-depth: also enforce record ownership.
	if err := h.db.RecordBelongsToUser(r.Context(), recordID, info.UserID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "envelope not found")
			return
		}
		log.Printf("vault: record ownership check error (record=%s user=%s): %v", recordID, info.UserID, err)
		writeErr(w, http.StatusInternalServerError, "failed to verify record ownership")
		return
	}

	env, err := h.db.GetEnvelope(r.Context(), recordID, deviceID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "envelope not found")
			return
		}
		log.Printf("vault: get envelope error (record=%s device=%s): %v", recordID, deviceID, err)
		writeErr(w, http.StatusInternalServerError, "failed to get envelope")
		return
	}

	h.auditAsync(info.UserID, info.TenantID, "envelope.fetch", &recordID, &deviceID, nil)
	// Never log envelope_ciphertext — only return it in the response.
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"envelope_ciphertext": base64.StdEncoding.EncodeToString(env.EnvelopeCiphertext),
		"envelope_nonce":      base64.StdEncoding.EncodeToString(env.EnvelopeNonce),
		"sealed_at":           env.SealedAt.Format(time.RFC3339),
	})
}

// PostEnvelope handles POST /vault/v1/records/{id}/envelopes — add envelope for a new device.
func (h *Handler) PostEnvelope(w http.ResponseWriter, r *http.Request) {
	info, ok := h.mustAuth(w, r)
	if !ok {
		return
	}
	recordID := chi.URLParam(r, "id")

	var body struct {
		DeviceID           string `json:"device_id"`
		EnvelopeCiphertext string `json:"envelope_ciphertext"` // base64
		EnvelopeNonce      string `json:"envelope_nonce"`      // base64
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.DeviceID == "" || body.EnvelopeCiphertext == "" || body.EnvelopeNonce == "" {
		writeErr(w, http.StatusBadRequest, "device_id, envelope_ciphertext, envelope_nonce required")
		return
	}

	ciphertext, err := b64Decode(body.EnvelopeCiphertext)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "envelope_ciphertext must be base64-encoded")
		return
	}
	nonce, err := b64Decode(body.EnvelopeNonce)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "envelope_nonce must be base64-encoded")
		return
	}

	// V05-F4: enforce that both the device and the record belong to the
	// authenticated user before creating an envelope. Without these checks a
	// caller could write an envelope binding ANOTHER user's record to ANOTHER
	// user's device, or seed their own ciphertext into a foreign envelope slot.
	if err := h.db.DeviceBelongsToUser(r.Context(), body.DeviceID, info.UserID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "device not found")
			return
		}
		log.Printf("vault: device ownership check error (device=%s user=%s): %v", body.DeviceID, info.UserID, err)
		writeErr(w, http.StatusInternalServerError, "failed to verify device ownership")
		return
	}
	if err := h.db.RecordBelongsToUser(r.Context(), recordID, info.UserID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "record not found")
			return
		}
		log.Printf("vault: record ownership check error (record=%s user=%s): %v", recordID, info.UserID, err)
		writeErr(w, http.StatusInternalServerError, "failed to verify record ownership")
		return
	}

	envelopeID, err := h.db.CreateEnvelope(r.Context(), recordID, body.DeviceID, ciphertext, nonce)
	if err != nil {
		log.Printf("vault: add envelope error (record=%s device=%s): %v", recordID, body.DeviceID, err)
		writeErr(w, http.StatusInternalServerError, "failed to add envelope")
		return
	}

	h.auditAsync(info.UserID, info.TenantID, "envelope.add", &recordID, &body.DeviceID, nil)
	writeJSON(w, http.StatusCreated, map[string]string{"envelope_id": envelopeID})
}

// DeleteRecord handles DELETE /vault/v1/records/{id} — soft-delete record + cascade wipe envelopes.
func (h *Handler) DeleteRecord(w http.ResponseWriter, r *http.Request) {
	info, ok := h.mustAuth(w, r)
	if !ok {
		return
	}
	recordID := chi.URLParam(r, "id")

	if err := h.db.RevokeRecord(r.Context(), recordID, info.UserID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "record not found")
			return
		}
		log.Printf("vault: revoke record error (record=%s user=%s): %v", recordID, info.UserID, err)
		writeErr(w, http.StatusInternalServerError, "failed to delete record")
		return
	}

	h.auditAsync(info.UserID, info.TenantID, "record.revoke", &recordID, nil, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// Health handles GET /health — liveness check.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
