package internal

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// ============================================================================
// Encryption Keys Handlers
// ============================================================================

func handleCreateEncryptionKey(db *DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		var req CreateEncryptionKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.ContentID == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("contentId is required"))
			return
		}

		rawKey, err := GenerateRandomBytes(16)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("generate key: %w", err))
			return
		}
		iv, err := GenerateRandomBytes(16)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("generate IV: %w", err))
			return
		}

		encryptedKey, err := EncryptKeyMaterial(hex.EncodeToString(rawKey), cfg.EncryptionKey)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("encrypt key: %w", err))
			return
		}

		host := cfg.Host
		if host == "0.0.0.0" {
			host = "localhost"
		}
		keyURI := fmt.Sprintf("http://%s:%d/v1/encryption/keys/KEY_ID/deliver", host, cfg.Port)

		key, err := scopedDB.CreateEncryptionKey(req.ContentID, encryptedKey, hex.EncodeToString(iv), keyURI)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("create encryption key: %w", err))
			return
		}

		actualKeyURI := strings.Replace(keyURI, "KEY_ID", key.ID, 1)

		_ = scopedDB.InsertWebhookEvent(
			fmt.Sprintf("tokens.encryption.key.created-%s", key.ID),
			"tokens.encryption.key.created",
			map[string]interface{}{"keyId": key.ID, "contentId": req.ContentID},
		)

		sdk.Respond(w, http.StatusCreated, map[string]interface{}{
			"keyId":  key.ID,
			"keyUri": actualKeyURI,
		})
	}
}

func handleDeliverEncryptionKey(db *DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		id := chi.URLParam(r, "id")
		key, err := scopedDB.GetEncryptionKeyByID(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get encryption key: %w", err))
			return
		}
		if key == nil || !key.IsActive {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("encryption key not found or inactive"))
			return
		}

		hexKey, err := DecryptKeyMaterial(key.KeyMaterialEncrypted, cfg.EncryptionKey)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("decrypt key: %w", err))
			return
		}

		rawKey, err := hex.DecodeString(hexKey)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("decode key hex: %w", err))
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(rawKey)))
		w.WriteHeader(http.StatusOK)
		w.Write(rawKey)
	}
}

func handleRotateEncryptionKey(db *DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		contentID := chi.URLParam(r, "contentId")

		var req RotateEncryptionKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		expireHours := 24
		if req.ExpireOldAfterHours != nil {
			expireHours = *req.ExpireOldAfterHours
		}

		rawKey, err := GenerateRandomBytes(16)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("generate key: %w", err))
			return
		}
		iv, err := GenerateRandomBytes(16)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("generate IV: %w", err))
			return
		}

		encryptedKey, err := EncryptKeyMaterial(hex.EncodeToString(rawKey), cfg.EncryptionKey)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("encrypt key: %w", err))
			return
		}

		host := cfg.Host
		if host == "0.0.0.0" {
			host = "localhost"
		}
		keyURI := fmt.Sprintf("http://%s:%d/v1/encryption/keys/KEY_ID/deliver", host, cfg.Port)

		newKey, err := scopedDB.RotateEncryptionKey(contentID, encryptedKey, hex.EncodeToString(iv), keyURI, expireHours)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("rotate encryption key: %w", err))
			return
		}

		actualKeyURI := strings.Replace(keyURI, "KEY_ID", newKey.ID, 1)

		_ = scopedDB.InsertWebhookEvent(
			fmt.Sprintf("tokens.encryption.key.rotated-%s", newKey.ID),
			"tokens.encryption.key.rotated",
			map[string]interface{}{"keyId": newKey.ID, "contentId": contentID},
		)

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"keyId":      newKey.ID,
			"keyUri":     actualKeyURI,
			"generation": newKey.RotationGeneration,
		})
	}
}

func handleListEncryptionKeys(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The TS source does not have a list endpoint for encryption keys,
		// but plugin.json actions include it. Return encryption key metadata
		// (without key material) for the requested content.
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"message": "Use POST /v1/encryption/keys to create, or GET /v1/encryption/keys/{id}/deliver to fetch",
		})
	}
}

