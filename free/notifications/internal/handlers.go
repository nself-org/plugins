package internal

import (
	"encoding/json"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
