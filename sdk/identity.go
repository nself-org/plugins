// Package sdk provides the nSelf plugin SDK.
//
// Identity provides per-plugin keypair management and request signing.
// Each plugin generates an Ed25519 keypair on first start (or loads from env).
// Inter-plugin requests include an X-Plugin-Signature header signed with the
// sender's private key. S43 wires the full trust chain and key distribution.
package sdk

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// PluginIdentity holds the Ed25519 keypair for a plugin instance.
type PluginIdentity struct {
	PluginName string
	PublicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

// GenerateIdentity creates a new Ed25519 keypair for the named plugin.
func GenerateIdentity(pluginName string) (*PluginIdentity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("identity: generate Ed25519 keypair: %w", err)
	}
	return &PluginIdentity{
		PluginName: pluginName,
		PublicKey:  pub,
		privateKey: priv,
	}, nil
}

// LoadOrGenerateIdentity loads from PLUGIN_PRIVATE_KEY env var (base64-encoded
// 64-byte raw Ed25519 private key), or generates a fresh keypair if absent.
func LoadOrGenerateIdentity(pluginName string) (*PluginIdentity, error) {
	raw := os.Getenv("PLUGIN_PRIVATE_KEY")
	if raw == "" {
		return GenerateIdentity(pluginName)
	}

	privBytes, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("identity: decode PLUGIN_PRIVATE_KEY: %w", err)
	}
	if len(privBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("identity: PLUGIN_PRIVATE_KEY must be %d bytes, got %d",
			ed25519.PrivateKeySize, len(privBytes))
	}

	priv := ed25519.PrivateKey(privBytes)
	pub := priv.Public().(ed25519.PublicKey)

	return &PluginIdentity{
		PluginName: pluginName,
		PublicKey:  pub,
		privateKey: priv,
	}, nil
}

// PublicKeyBase64 returns the public key as a base64-encoded string for sharing.
func (id *PluginIdentity) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(id.PublicKey)
}

// Sign signs the message with the plugin's private key and returns a base64-encoded signature.
func (id *PluginIdentity) Sign(message []byte) string {
	sig := ed25519.Sign(id.privateKey, message)
	return base64.StdEncoding.EncodeToString(sig)
}

// signatureMessage constructs the canonical message that is signed/verified for
// a given request. Format: "{plugin-name}\n{unix-timestamp}\n{HTTP-method}\n{path}"
func signatureMessage(pluginName, timestamp, method, path string) []byte {
	return []byte(pluginName + "\n" + timestamp + "\n" + method + "\n" + path)
}

// SignRequest adds X-Plugin-Id, X-Plugin-Timestamp, and X-Plugin-Signature
// headers to the outgoing request. The signature covers:
//
//	"{plugin-name}\n{unix-timestamp}\n{HTTP-method}\n{url-path}"
func (id *PluginIdentity) SignRequest(r *http.Request) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	path := r.URL.Path
	if path == "" {
		path = "/"
	}

	msg := signatureMessage(id.PluginName, ts, r.Method, path)
	sig := id.Sign(msg)

	r.Header.Set("X-Plugin-Id", id.PluginName)
	r.Header.Set("X-Plugin-Timestamp", ts)
	r.Header.Set("X-Plugin-Signature", sig)
}

// VerifySignedRequest validates the X-Plugin-Signature on an incoming request.
//
// pubKeyBase64 is the sender's public key as a base64-encoded string (32 raw
// bytes). For S43 this will come from a registry; for now callers pass it
// explicitly or set PLUGIN_TRUSTED_KEYS env var.
//
// Returns an error if the signature is missing, the timestamp is expired
// (>5 minutes old), or the signature is invalid.
func VerifySignedRequest(r *http.Request, pubKeyBase64 string) error {
	pluginID := r.Header.Get("X-Plugin-Id")
	if pluginID == "" {
		return fmt.Errorf("identity: missing X-Plugin-Id header")
	}

	tsStr := r.Header.Get("X-Plugin-Timestamp")
	if tsStr == "" {
		return fmt.Errorf("identity: missing X-Plugin-Timestamp header")
	}

	sigB64 := r.Header.Get("X-Plugin-Signature")
	if sigB64 == "" {
		return fmt.Errorf("identity: missing X-Plugin-Signature header")
	}

	// Validate timestamp (replay protection: reject if >5 minutes old).
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("identity: invalid X-Plugin-Timestamp value")
	}
	age := time.Since(time.Unix(ts, 0))
	if age > 5*time.Minute {
		return fmt.Errorf("identity: request timestamp expired (%s old)", age.Round(time.Second))
	}
	if age < -30*time.Second {
		// Guard against far-future timestamps (clock skew tolerance: 30s).
		return fmt.Errorf("identity: request timestamp is too far in the future")
	}

	// Decode the sender's public key.
	pubBytes, err := base64.StdEncoding.DecodeString(pubKeyBase64)
	if err != nil {
		return fmt.Errorf("identity: decode public key: %w", err)
	}
	if len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("identity: public key must be %d bytes, got %d",
			ed25519.PublicKeySize, len(pubBytes))
	}
	pub := ed25519.PublicKey(pubBytes)

	// Decode the signature.
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("identity: decode signature: %w", err)
	}

	// Reconstruct the signed message.
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	msg := signatureMessage(pluginID, tsStr, r.Method, path)

	if !ed25519.Verify(pub, msg, sig) {
		return fmt.Errorf("identity: signature verification failed")
	}

	return nil
}

// RequirePluginSignature returns an HTTP middleware that verifies the
// X-Plugin-Signature header on incoming requests.
//
// The trusted public key is read from the environment variable:
//
//	PLUGIN_TRUSTED_KEY_{PLUGIN_NAME_UPPER}
//
// where PLUGIN_NAME_UPPER is the sender plugin name in upper-case with hyphens
// replaced by underscores (e.g. "my-plugin" → "PLUGIN_TRUSTED_KEY_MY_PLUGIN").
//
// If the env var is not set, the middleware passes through (graceful
// degradation for the S43 rollout). Once S43 ships, this becomes strict.
func RequirePluginSignature(pluginName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Derive the env var name from the sending plugin's X-Plugin-Id.
			// If the header is absent we fall back to the configured pluginName.
			senderID := r.Header.Get("X-Plugin-Id")
			if senderID == "" {
				senderID = pluginName
			}

			envKey := "PLUGIN_TRUSTED_KEY_" + strings.ToUpper(
				strings.ReplaceAll(senderID, "-", "_"),
			)

			pubKeyBase64 := os.Getenv(envKey)
			if pubKeyBase64 == "" {
				// Graceful degradation: key not configured yet — let request through.
				// S43 will make this strict.
				next.ServeHTTP(w, r)
				return
			}

			if err := VerifySignedRequest(r, pubKeyBase64); err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
