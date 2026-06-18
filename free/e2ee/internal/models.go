package internal

import "time"

// Request/response payload types for the e2ee key directory.
//
// All key fields are base64-encoded PUBLIC key material. There are NO private
// key fields anywhere in this file by design — the server never sees a secret.

// RegisterIdentityRequest publishes a device's long-term PUBLIC identity key.
type RegisterIdentityRequest struct {
	UserID            string `json:"user_id"`
	DeviceID          string `json:"device_id"`
	IdentityKeyPublic string `json:"identity_key_public"` // base64 Ed25519 public key
	RegistrationID    int    `json:"registration_id"`
}

// SignedPreKeyUpload publishes a signed prekey (PUBLIC + Ed25519 signature).
type SignedPreKeyUpload struct {
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id"`
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"` // base64 X25519 public key
	Signature string `json:"signature"`  // base64 Ed25519 signature over PublicKey
}

// OneTimePreKey is a single classic X25519 one-time PUBLIC prekey.
type OneTimePreKey struct {
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"` // base64 X25519 public key
}

// KyberPreKey is a single Kyber-1024 (ML-KEM-1024) one-time PUBLIC prekey.
type KyberPreKey struct {
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"` // base64 ML-KEM-1024 public key
	Signature string `json:"signature"`  // base64 Ed25519 signature over PublicKey
}

// UploadOneTimePreKeysRequest batch-publishes classic + Kyber one-time prekeys.
// The identity key used to verify Kyber signatures is read from the STORED row
// (np_e2ee_identity_keys), never from the request body — so there is no
// client-supplied identity_key_public field here (CR-C MED fix).
type UploadOneTimePreKeysRequest struct {
	UserID       string          `json:"user_id"`
	DeviceID     string          `json:"device_id"`
	OneTimeKeys  []OneTimePreKey `json:"one_time_keys"`
	KyberPreKeys []KyberPreKey   `json:"kyber_prekeys"`
}

// PreKeyBundleResponse is the bundle returned to an X3DH+PQ initiator.
// One classic one-time prekey and one Kyber prekey are consumed atomically.
// Any field may be nil if that key type is exhausted (graceful degradation).
type PreKeyBundleResponse struct {
	UserID            string `json:"user_id"`
	DeviceID          string `json:"device_id"`
	RegistrationID    int    `json:"registration_id"`
	IdentityKeyPublic string `json:"identity_key_public"`

	SignedPreKeyID        int    `json:"signed_prekey_id"`
	SignedPreKeyPublic    string `json:"signed_prekey_public"`
	SignedPreKeySignature string `json:"signed_prekey_signature"`

	OneTimePreKeyID     *int    `json:"one_time_prekey_id,omitempty"`
	OneTimePreKeyPublic *string `json:"one_time_prekey_public,omitempty"`

	KyberPreKeyID        *int    `json:"kyber_prekey_id,omitempty"`
	KyberPreKeyPublic    *string `json:"kyber_prekey_public,omitempty"`
	KyberPreKeySignature *string `json:"kyber_prekey_signature,omitempty"`
}

// ReplenishStatus reports remaining unconsumed prekey counts for a device.
type ReplenishStatus struct {
	UserID          string `json:"user_id"`
	DeviceID        string `json:"device_id"`
	OneTimeRemaining int   `json:"one_time_remaining"`
	KyberRemaining   int   `json:"kyber_remaining"`
	NeedsReplenish   bool  `json:"needs_replenish"`
}

// SafetyNumberRequest posts a computed safety number + verification flag.
type SafetyNumberRequest struct {
	UserID       string `json:"user_id"`
	PeerUserID   string `json:"peer_user_id"`
	SafetyNumber string `json:"safety_number"`
	IsVerified   bool   `json:"is_verified"`
}

// VerificationState is the per-peer verification state returned to a client.
type VerificationState struct {
	UserID     string    `json:"user_id"`
	PeerUserID string    `json:"peer_user_id"`
	State      string    `json:"state"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// AuditEntry is one append-only security event row.
type AuditEntry struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	EventType string    `json:"event_type"`
	CreatedAt time.Time `json:"created_at"`
}
