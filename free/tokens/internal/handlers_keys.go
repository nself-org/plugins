package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// ============================================================================
// Signing Keys Handlers
// ============================================================================

func handleCreateKey(db *DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		var req CreateSigningKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("name is required"))
			return
		}

		algorithm := req.Algorithm
		if algorithm == "" {
			algorithm = cfg.SigningAlgorithm
		}

		rawKey, err := GenerateRandomHex(32)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("generate key: %w", err))
			return
		}

		encryptedKey, err := EncryptKeyMaterial(rawKey, cfg.EncryptionKey)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("encrypt key: %w", err))
			return
		}

		key, err := scopedDB.CreateSigningKey(req.Name, algorithm, encryptedKey)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("create signing key: %w", err))
			return
		}

		sdk.Respond(w, http.StatusCreated, map[string]interface{}{
			"id":        key.ID,
			"name":      key.Name,
			"algorithm": key.Algorithm,
			"isActive":  key.IsActive,
			"createdAt": key.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
}

func handleListKeys(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		keys, err := scopedDB.ListSigningKeys()
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("list signing keys: %w", err))
			return
		}

		result := make([]map[string]interface{}, 0, len(keys))
		for _, k := range keys {
			entry := map[string]interface{}{
				"id":          k.ID,
				"name":        k.Name,
				"algorithm":   k.Algorithm,
				"isActive":    k.IsActive,
				"rotatedFrom": k.RotatedFrom,
				"createdAt":   k.CreatedAt.UTC().Format(time.RFC3339),
			}
			if k.RotatedAt != nil {
				entry["rotatedAt"] = k.RotatedAt.UTC().Format(time.RFC3339)
			} else {
				entry["rotatedAt"] = nil
			}
			if k.ExpiresAt != nil {
				entry["expiresAt"] = k.ExpiresAt.UTC().Format(time.RFC3339)
			} else {
				entry["expiresAt"] = nil
			}
			result = append(result, entry)
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{"keys": result})
	}
}

func handleRotateKey(db *DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		var req struct {
			KeyID               string `json:"keyId"`
			ExpireOldAfterHours *int   `json:"expireOldAfterHours,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.KeyID == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("keyId is required"))
			return
		}

		expireHours := 24
		if req.ExpireOldAfterHours != nil {
			expireHours = *req.ExpireOldAfterHours
		}

		rawKey, err := GenerateRandomHex(32)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("generate key: %w", err))
			return
		}
		encryptedKey, err := EncryptKeyMaterial(rawKey, cfg.EncryptionKey)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("encrypt key: %w", err))
			return
		}

		newKey, err := scopedDB.RotateSigningKey(req.KeyID, encryptedKey, expireHours)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				sdk.Error(w, http.StatusNotFound, fmt.Errorf("signing key not found"))
			} else {
				sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("rotate key: %w", err))
			}
			return
		}

		_ = scopedDB.InsertWebhookEvent(
			fmt.Sprintf("tokens.key.rotated-%s", newKey.ID),
			"tokens.key.rotated",
			map[string]interface{}{"oldKeyId": req.KeyID, "newKeyId": newKey.ID},
		)

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"id":          newKey.ID,
			"name":        newKey.Name,
			"algorithm":   newKey.Algorithm,
			"isActive":    newKey.IsActive,
			"rotatedFrom": newKey.RotatedFrom,
			"createdAt":   newKey.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
}

func handleDeleteKey(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		id := chi.URLParam(r, "id")
		if err := scopedDB.DeactivateSigningKey(id); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("deactivate key: %w", err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

