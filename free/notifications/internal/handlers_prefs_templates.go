package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

func handleUpdatePreferences(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "user_id")
		if userID == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}

		var req UpdatePreferencesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		// Build preference with defaults for nil fields.
		pref := Preference{
			UserID:       userID,
			EmailEnabled: true,
			PushEnabled:  true,
			SMSEnabled:   true,
			QuietStart:   req.QuietStart,
			QuietEnd:     req.QuietEnd,
			Channels:     req.Channels,
		}
		if req.EmailEnabled != nil {
			pref.EmailEnabled = *req.EmailEnabled
		}
		if req.PushEnabled != nil {
			pref.PushEnabled = *req.PushEnabled
		}
		if req.SMSEnabled != nil {
			pref.SMSEnabled = *req.SMSEnabled
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := UpsertPreference(ctx, pool, pref); err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		sdk.Respond(w, http.StatusOK, pref)
	}
}

func handleGetPreferences(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "user_id")
		if userID == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		pref, err := GetPreference(ctx, pool, userID)
		if err != nil {
			// Return defaults if no preferences are stored yet.
			sdk.Respond(w, http.StatusOK, Preference{
				UserID:       userID,
				EmailEnabled: true,
				PushEnabled:  true,
				SMSEnabled:   true,
				Channels:     json.RawMessage("{}"),
			})
			return
		}

		sdk.Respond(w, http.StatusOK, pref)
	}
}

func handleCreateTemplate(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateTemplateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if req.Name == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if req.Channel == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "channel is required"})
			return
		}

		id := uuid.New().String()

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		err := InsertTemplate(ctx, pool, id, req.Name, req.Channel, req.SubjectTemplate, req.BodyTemplate)
		if err != nil {
			sdk.Respond(w, http.StatusConflict, map[string]string{"error": "template name already exists or insert failed: " + err.Error()})
			return
		}

		sdk.Respond(w, http.StatusCreated, map[string]interface{}{
			"id":               id,
			"name":             req.Name,
			"channel":          req.Channel,
			"subject_template": req.SubjectTemplate,
			"body_template":    req.BodyTemplate,
		})
	}
}

func handleListTemplates(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		templates, err := ListTemplates(ctx, pool)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if templates == nil {
			templates = []Template{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"templates": templates,
		})
	}
}
