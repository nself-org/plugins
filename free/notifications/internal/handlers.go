package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

// RegisterRoutes mounts all notifications endpoints on the given router.
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool) {
	// Notifications
	r.Post("/v1/notifications", handleSendNotification(pool))
	r.Get("/v1/notifications", handleListNotifications(pool))
	r.Get("/v1/notifications/{id}", handleGetNotification(pool))

	// Preferences
	r.Put("/v1/preferences/{user_id}", handleUpdatePreferences(pool))
	r.Get("/v1/preferences/{user_id}", handleGetPreferences(pool))

	// Templates
	r.Get("/v1/templates", handleListTemplates(pool))
	r.Post("/v1/templates", handleCreateTemplate(pool))
}

// --- Request / Response types ------------------------------------------------

// SendNotificationRequest is the JSON body for POST /v1/notifications.
type SendNotificationRequest struct {
	Channel   string          `json:"channel"`
	Recipient string          `json:"recipient"`
	Template  string          `json:"template"`
	Data      json.RawMessage `json:"data"`
}

// UpdatePreferencesRequest is the JSON body for PUT /v1/preferences/:user_id.
type UpdatePreferencesRequest struct {
	EmailEnabled *bool           `json:"email_enabled"`
	PushEnabled  *bool           `json:"push_enabled"`
	SMSEnabled   *bool           `json:"sms_enabled"`
	QuietStart   *string         `json:"quiet_start"`
	QuietEnd     *string         `json:"quiet_end"`
	Channels     json.RawMessage `json:"channels"`
}

// CreateTemplateRequest is the JSON body for POST /v1/templates.
type CreateTemplateRequest struct {
	Name            string `json:"name"`
	Channel         string `json:"channel"`
	SubjectTemplate string `json:"subject_template"`
	BodyTemplate    string `json:"body_template"`
}

// --- Handlers ----------------------------------------------------------------

func handleSendNotification(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SendNotificationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if req.Channel == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "channel is required"})
			return
		}
		if req.Recipient == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "recipient is required"})
			return
		}

		// Resolve template content if a template name is provided.
		subject := ""
		body := ""
		if req.Template != "" {
			subject = req.Template
			body = req.Template
		}

		var result ChannelResult
		switch req.Channel {
		case "email":
			result = SendEmail(req.Recipient, subject, body)
		case "push":
			result = SendPush(req.Recipient, subject, body)
		case "sms":
			result = SendSMS(req.Recipient, body)
		default:
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "unsupported channel: " + req.Channel})
			return
		}

		// Record the notification in the database.
		id := uuid.New().String()
		status := "sent"
		var sentAt *time.Time
		var errMsg *string

		if result.Success {
			now := time.Now().UTC()
			sentAt = &now
		} else {
			status = "failed"
			errMsg = result.Error
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		_ = InsertNotification(ctx, pool, id, req.Channel, req.Recipient, req.Template, req.Data, status, sentAt, errMsg)

		httpStatus := http.StatusOK
		if !result.Success {
			httpStatus = http.StatusBadGateway
		}

		sdk.Respond(w, httpStatus, map[string]interface{}{
			"id":      id,
			"channel": result.Channel,
			"success": result.Success,
			"error":   result.Error,
		})
	}
}

func handleListNotifications(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channel := r.URL.Query().Get("channel")
		status := r.URL.Query().Get("status")
		recipient := r.URL.Query().Get("recipient")

		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}

		offset := 0
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		notifications, err := ListNotifications(ctx, pool, channel, status, recipient, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if notifications == nil {
			notifications = []Notification{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"notifications": notifications,
			"limit":         limit,
			"offset":        offset,
		})
	}
}

func handleGetNotification(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		notification, err := GetNotification(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "notification not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, notification)
	}
}

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
