package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

// Size-cap exception: single-responsibility HTTP route handler — 71L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
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

// Size-cap exception: single-responsibility HTTP route handler — 65L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
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
