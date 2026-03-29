package internal

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// RegisterRoutes mounts all tokens API routes on the given router.
func RegisterRoutes(r chi.Router, db *DB, cfg *Config) {
	r.Route("/v1", func(r chi.Router) {
		// Token issuance and validation
		r.Post("/tokens/issue", handleIssueToken(db, cfg))
		r.Post("/tokens/validate", handleValidateToken(db, cfg))
		r.Post("/tokens/revoke", handleRevokeToken(db))
		r.Get("/tokens", handleListTokens(db))

		// Signing keys
		r.Get("/keys", handleListKeys(db))
		r.Post("/keys", handleCreateKey(db, cfg))
		r.Post("/keys/rotate", handleRotateKey(db, cfg))
		r.Delete("/keys/{id}", handleDeleteKey(db))

		// Entitlements
		r.Get("/entitlements", handleListEntitlements(db))
		r.Post("/entitlements", handleCreateEntitlement(db))
		r.Get("/entitlements/{id}", handleGetEntitlement(db))
		r.Delete("/entitlements/{id}", handleDeleteEntitlement(db))
		r.Post("/entitlements/check", handleCheckEntitlement(db, cfg))

		// HLS encryption keys
		r.Get("/encryption/keys", handleListEncryptionKeys(db))
		r.Post("/encryption/keys", handleCreateEncryptionKey(db, cfg))
		r.Get("/encryption/keys/{id}/deliver", handleDeliverEncryptionKey(db, cfg))
		r.Post("/encryption/keys/{contentId}/rotate", handleRotateEncryptionKey(db, cfg))

		// Stats
		r.Get("/stats", handleStats(db))
	})
}

// getSourceAccountID extracts the source account from the X-Source-Account-Id header.
func getSourceAccountID(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-Id"); v != "" {
		return v
	}
	return "primary"
}

// ============================================================================
// Token Issuance Handlers
// ============================================================================

func handleIssueToken(db *DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		var req IssueTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.UserID == "" || req.ContentID == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("userId and contentId are required"))
			return
		}

		tokenType := req.TokenType
		if tokenType == "" {
			tokenType = "playback"
		}

		// Check entitlements if enabled
		if cfg.DefaultEntitlementCheck {
			entitlement, err := scopedDB.CheckEntitlement(req.UserID, req.ContentID, tokenType)
			if err != nil {
				sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("entitlement check failed: %w", err))
				return
			}
			if entitlement == nil {
				hasAny, err := scopedDB.HasAnyEntitlements(req.UserID)
				if err != nil {
					sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("entitlement lookup failed: %w", err))
					return
				}
				if hasAny || !cfg.AllowAllIfNoEntitlements {
					_ = scopedDB.InsertWebhookEvent(
						fmt.Sprintf("tokens.access.denied-%s-%s-%d", req.UserID, req.ContentID, time.Now().UnixMilli()),
						"tokens.access.denied",
						map[string]interface{}{"userId": req.UserID, "contentId": req.ContentID, "reason": "no_entitlement"},
					)
					sdk.Error(w, http.StatusForbidden, fmt.Errorf("access denied: no valid entitlement"))
					return
				}
			}
		}

		// Get active signing key
		signingKey, err := scopedDB.GetActiveSigningKey()
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("signing key lookup failed: %w", err))
			return
		}
		if signingKey == nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("no active signing key configured; create one first"))
			return
		}

		keyMaterial, err := DecryptKeyMaterial(signingKey.KeyMaterialEncrypted, cfg.EncryptionKey)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("decrypt signing key: %w", err))
			return
		}

		ttl := cfg.DefaultTTLSeconds
		if req.TTLSeconds != nil && *req.TTLSeconds > 0 {
			ttl = *req.TTLSeconds
		}
		if ttl > cfg.MaxTTLSeconds {
			ttl = cfg.MaxTTLSeconds
		}
		expiresAt := time.Now().Add(time.Duration(ttl) * time.Second)

		payload := map[string]interface{}{
			"sub":  req.UserID,
			"cid":  req.ContentID,
			"typ":  tokenType,
			"exp":  expiresAt.Unix(),
			"iat":  time.Now().Unix(),
			"perm": req.Permissions,
		}
		if payload["perm"] == nil {
			payload["perm"] = map[string]interface{}{}
		}
		if req.DeviceID != "" {
			payload["did"] = req.DeviceID
		}
		if req.IPRestriction != "" {
			payload["ip"] = req.IPRestriction
		}
		if req.ContentType != "" {
			payload["ctype"] = req.ContentType
		}

		token, err := GenerateToken(payload, keyMaterial)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("generate token: %w", err))
			return
		}
		tokenHash := HashToken(token)

		var deviceID *string
		if req.DeviceID != "" {
			deviceID = &req.DeviceID
		}
		var contentType *string
		if req.ContentType != "" {
			contentType = &req.ContentType
		}
		var ipAddr *string
		if req.IPRestriction != "" {
			ipAddr = &req.IPRestriction
		}
		perms := req.Permissions
		if perms == nil {
			perms = map[string]interface{}{}
		}

		issued, err := scopedDB.InsertIssuedToken(InsertTokenParams{
			TokenHash:    tokenHash,
			TokenType:    tokenType,
			SigningKeyID: signingKey.ID,
			UserID:       req.UserID,
			DeviceID:     deviceID,
			ContentID:    req.ContentID,
			ContentType:  contentType,
			Permissions:  perms,
			IPAddress:    ipAddr,
			ExpiresAt:    expiresAt,
		})
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("store issued token: %w", err))
			return
		}

		_ = scopedDB.InsertWebhookEvent(
			fmt.Sprintf("tokens.issued-%s", issued.ID),
			"tokens.issued",
			map[string]interface{}{"tokenId": issued.ID, "userId": req.UserID, "contentId": req.ContentID},
		)

		sdk.Respond(w, http.StatusOK, IssueTokenResponse{
			Token:     token,
			ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
			TokenID:   issued.ID,
		})
	}
}

func handleValidateToken(db *DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		var req ValidateTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Token == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("token is required"))
			return
		}

		tokenHash := HashToken(req.Token)
		issued, err := scopedDB.GetIssuedTokenByHash(tokenHash)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("token lookup failed: %w", err))
			return
		}

		if issued == nil {
			sdk.Respond(w, http.StatusOK, ValidateTokenResponse{Valid: false})
			return
		}

		if issued.Revoked {
			sdk.Respond(w, http.StatusOK, ValidateTokenResponse{Valid: false})
			return
		}

		if time.Now().After(issued.ExpiresAt) {
			sdk.Respond(w, http.StatusOK, ValidateTokenResponse{Valid: false})
			return
		}

		// Check content ID restriction
		if req.ContentID != "" && issued.ContentID != req.ContentID {
			sdk.Respond(w, http.StatusOK, ValidateTokenResponse{Valid: false})
			return
		}

		// Check IP restriction
		if issued.IPAddress != nil && req.IPAddress != "" && *issued.IPAddress != req.IPAddress {
			sdk.Respond(w, http.StatusOK, ValidateTokenResponse{Valid: false})
			return
		}

		_ = scopedDB.UpdateTokenLastUsed(issued.ID)

		_ = scopedDB.InsertWebhookEvent(
			fmt.Sprintf("tokens.validated-%s-%d", issued.ID, time.Now().UnixMilli()),
			"tokens.validated",
			map[string]interface{}{"tokenId": issued.ID, "userId": issued.UserID},
		)

		var perms map[string]interface{}
		if len(issued.Permissions) > 0 {
			_ = json.Unmarshal(issued.Permissions, &perms)
		}

		sdk.Respond(w, http.StatusOK, ValidateTokenResponse{
			Valid:       true,
			UserID:      issued.UserID,
			ContentID:   issued.ContentID,
			Permissions: perms,
			ExpiresAt:   issued.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
}

func handleRevokeToken(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		// Try to decode as a general revoke request that could contain tokenId, userId, or contentId
		var raw map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		reason, _ := raw["reason"].(string)

		// Revoke by userId
		if userID, ok := raw["userId"].(string); ok && userID != "" {
			count, err := scopedDB.RevokeUserTokens(userID, reason)
			if err != nil {
				sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("revoke user tokens: %w", err))
				return
			}
			_ = scopedDB.InsertWebhookEvent(
				fmt.Sprintf("tokens.revoked-user-%s-%d", userID, time.Now().UnixMilli()),
				"tokens.revoked",
				map[string]interface{}{"userId": userID, "reason": reason, "count": count},
			)
			sdk.Respond(w, http.StatusOK, map[string]interface{}{"revoked": count, "userId": userID})
			return
		}

		// Revoke by contentId
		if contentID, ok := raw["contentId"].(string); ok && contentID != "" {
			count, err := scopedDB.RevokeContentTokens(contentID, reason)
			if err != nil {
				sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("revoke content tokens: %w", err))
				return
			}
			_ = scopedDB.InsertWebhookEvent(
				fmt.Sprintf("tokens.revoked-content-%s-%d", contentID, time.Now().UnixMilli()),
				"tokens.revoked",
				map[string]interface{}{"contentId": contentID, "reason": reason, "count": count},
			)
			sdk.Respond(w, http.StatusOK, map[string]interface{}{"revoked": count, "contentId": contentID})
			return
		}

		// Revoke by tokenId
		tokenID, _ := raw["tokenId"].(string)
		if tokenID == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("tokenId, userId, or contentId is required"))
			return
		}

		if err := scopedDB.RevokeToken(tokenID, reason); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("revoke token: %w", err))
			return
		}
		_ = scopedDB.InsertWebhookEvent(
			fmt.Sprintf("tokens.revoked-%s", tokenID),
			"tokens.revoked",
			map[string]interface{}{"tokenId": tokenID, "reason": reason},
		)
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"revoked": true, "tokenId": tokenID})
	}
}

func handleListTokens(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The TS source does not expose a list tokens endpoint, but plugin.json
		// implies GET /v1/tokens. Return a simple message for now.
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"message": "Use POST /v1/tokens/validate to check token status",
		})
	}
}

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

// ============================================================================
// Entitlements Handlers
// ============================================================================

func handleCheckEntitlement(db *DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		var req CheckEntitlementRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.UserID == "" || req.ContentID == "" || req.EntitlementType == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("userId, contentId, and entitlementType are required"))
			return
		}

		entitlement, err := scopedDB.CheckEntitlement(req.UserID, req.ContentID, req.EntitlementType)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("check entitlement: %w", err))
			return
		}

		if entitlement != nil {
			resp := CheckEntitlementResponse{
				Allowed: true,
				Reason:  "entitlement_active",
			}
			// Extract restrictions from metadata
			var meta map[string]interface{}
			if len(entitlement.Metadata) > 0 {
				_ = json.Unmarshal(entitlement.Metadata, &meta)
				if restrictions, ok := meta["restrictions"].(map[string]interface{}); ok {
					resp.Restrictions = restrictions
				}
			}
			if entitlement.ExpiresAt != nil {
				resp.ExpiresAt = entitlement.ExpiresAt.UTC().Format(time.RFC3339)
			}
			sdk.Respond(w, http.StatusOK, resp)
			return
		}

		// Check if user has any entitlements at all
		hasAny, err := scopedDB.HasAnyEntitlements(req.UserID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("entitlement lookup: %w", err))
			return
		}
		if !hasAny && cfg.AllowAllIfNoEntitlements {
			sdk.Respond(w, http.StatusOK, CheckEntitlementResponse{
				Allowed: true,
				Reason:  "no_entitlements_mode",
			})
			return
		}

		_ = scopedDB.InsertWebhookEvent(
			fmt.Sprintf("tokens.access.denied-%s-%s-%d", req.UserID, req.ContentID, time.Now().UnixMilli()),
			"tokens.access.denied",
			map[string]interface{}{"userId": req.UserID, "contentId": req.ContentID, "entitlementType": req.EntitlementType},
		)

		sdk.Respond(w, http.StatusOK, CheckEntitlementResponse{
			Allowed: false,
			Reason:  "no_valid_entitlement",
		})
	}
}

func handleCreateEntitlement(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		var req GrantEntitlementRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.UserID == "" || req.ContentID == "" || req.EntitlementType == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("userId, contentId, and entitlementType are required"))
			return
		}

		var contentType *string
		if req.ContentType != "" {
			contentType = &req.ContentType
		}

		var expiresAt *time.Time
		if req.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, req.ExpiresAt)
			if err != nil {
				sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid expiresAt format: %w", err))
				return
			}
			expiresAt = &t
		}

		metadata := req.Metadata
		if metadata == nil {
			metadata = map[string]interface{}{}
		}

		entitlement, err := scopedDB.GrantEntitlement(GrantEntitlementParams{
			UserID:          req.UserID,
			ContentID:       req.ContentID,
			ContentType:     contentType,
			EntitlementType: req.EntitlementType,
			ExpiresAt:       expiresAt,
			Metadata:        metadata,
			GrantedBy:       "api",
		})
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("grant entitlement: %w", err))
			return
		}

		_ = scopedDB.InsertWebhookEvent(
			fmt.Sprintf("tokens.entitlement.granted-%s", entitlement.ID),
			"tokens.entitlement.granted",
			map[string]interface{}{"entitlementId": entitlement.ID, "userId": req.UserID, "contentId": req.ContentID},
		)

		sdk.Respond(w, http.StatusCreated, entitlement)
	}
}

func handleGetEntitlement(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		id := chi.URLParam(r, "id")
		entitlement, err := scopedDB.GetEntitlementByID(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get entitlement: %w", err))
			return
		}
		if entitlement == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("entitlement not found"))
			return
		}

		sdk.Respond(w, http.StatusOK, entitlement)
	}
}

func handleDeleteEntitlement(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		id := chi.URLParam(r, "id")
		deleted, err := scopedDB.DeleteEntitlementByID(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("delete entitlement: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("entitlement not found"))
			return
		}

		_ = scopedDB.InsertWebhookEvent(
			fmt.Sprintf("tokens.entitlement.deleted-%s-%d", id, time.Now().UnixMilli()),
			"tokens.entitlement.deleted",
			map[string]interface{}{"entitlementId": id},
		)

		sdk.Respond(w, http.StatusOK, map[string]interface{}{"deleted": true})
	}
}

func handleListEntitlements(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		userID := r.URL.Query().Get("userId")
		if userID == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("userId query parameter is required"))
			return
		}

		var contentType *string
		if ct := r.URL.Query().Get("contentType"); ct != "" {
			contentType = &ct
		}

		activeOnly := r.URL.Query().Get("active") != "false"

		entitlements, err := scopedDB.ListUserEntitlements(userID, contentType, activeOnly)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("list entitlements: %w", err))
			return
		}
		if entitlements == nil {
			entitlements = []Entitlement{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{"entitlements": entitlements})
	}
}

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

// ============================================================================
// Stats Handler
// ============================================================================

func handleStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		stats, err := scopedDB.GetStats()
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get stats: %w", err))
			return
		}

		sdk.Respond(w, http.StatusOK, stats)
	}
}
