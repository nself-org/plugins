package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	sdk "github.com/nself-org/plugin-sdk"
)

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
