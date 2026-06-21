package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

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

