package internal

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	sdk "github.com/nself-org/plugin-sdk"
)

// RegisterRoutes mounts all push plugin routes onto the given router.
// Routes:
//   POST /push/dispatch     — Hasura event trigger endpoint
//   POST /push/devices      — register / update a device token
//   GET  /health            — health check
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool, dispatcher *Dispatcher) {
	r.Post("/push/dispatch", handleDispatch(pool, dispatcher))
	r.Post("/push/devices", handleRegisterDevice(pool))
	r.Get("/health", handleHealth())
}

// --- Dispatch handler ---

// hasuraEventPayload is the shape Hasura sends for INSERT event triggers.
// We only need the "new" row from np_push_outbox.
type hasuraEventPayload struct {
	Event struct {
		Op   string `json:"op"` // "INSERT", "UPDATE", "DELETE", "MANUAL"
		Data struct {
			New *outboxEventRow `json:"new"`
		} `json:"data"`
	} `json:"event"`
}

// outboxEventRow mirrors the np_push_outbox columns we need from the Hasura event.
type outboxEventRow struct {
	ID          string          `json:"id"`
	DeviceToken string          `json:"device_token"`
	Platform    string          `json:"platform"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	Attempts    int             `json:"attempts"`
}

// handleDispatch processes Hasura event trigger POSTs for np_push_outbox INSERTs.
// It is idempotent: duplicate events (at-least-once delivery from Hasura) are
// handled by the dedupe_hash unique constraint in the DB and the status check below.
func handleDispatch(pool *pgxpool.Pool, dispatcher *Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var ev hasuraEventPayload
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}

		if ev.Event.Op != "INSERT" && ev.Event.Op != "MANUAL" {
			// Only handle new-row events; silently ignore updates/deletes.
			sdk.Respond(w, http.StatusOK, map[string]string{"status": "ignored", "op": ev.Event.Op})
			return
		}

		row := ev.Event.Data.New
		if row == nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "event.data.new is null"})
			return
		}

		// Idempotency: skip if this row is already being processed or completed.
		if row.Status != StatusPending {
			sdk.Respond(w, http.StatusOK, map[string]string{"status": "skipped", "reason": "row not in pending state"})
			return
		}

		// SSRF guard: device_token must not be a URL. APNs and FCM tokens are
		// opaque hex/base64 strings — rejecting tokens that look like HTTP URLs
		// prevents an attacker from using the outbox as an SSRF proxy.
		if isURL(row.DeviceToken) {
			log.Printf("[push] SECURITY: rejecting dispatch for outbox %s — device_token looks like a URL (potential SSRF)", row.ID)
			errMsg := "invalid device_token: must not be a URL"
			if updErr := UpdateOutboxStatus(ctx, pool, row.ID, StatusFailed, 0, &errMsg); updErr != nil {
				log.Printf("[push] WARNING: failed to mark SSRF-rejected row as failed: %v", updErr)
			}
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}

		job := DispatchJob{
			OutboxID:    row.ID,
			DeviceToken: row.DeviceToken,
			Platform:    row.Platform,
			Payload:     row.Payload,
			Attempts:    row.Attempts,
		}

		// Dispatch asynchronously so we can return 200 to Hasura immediately.
		// Hasura has a short event-trigger timeout; the actual delivery happens
		// in a goroutine. Status is written back to the DB row.
		go func() {
			bgCtx := r.Context()
			if err := dispatcher.Dispatch(bgCtx, job); err != nil {
				log.Printf("[push] dispatch failed for outbox %s: %v", row.ID, err)
			}
		}()

		sdk.Respond(w, http.StatusOK, map[string]string{"status": "accepted", "id": row.ID})
	}
}

// --- Device registration handler ---

type registerDeviceRequest struct {
	DeviceToken string  `json:"device_token"`
	Platform    string  `json:"platform"`
	AppID       string  `json:"app_id"`
	UserID      *string `json:"user_id,omitempty"`
}

// handleRegisterDevice upserts a device token into np_push_devices.
// Called by client apps when they receive a new APNs/FCM registration token.
func handleRegisterDevice(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var req registerDeviceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}

		if req.DeviceToken == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "device_token is required"})
			return
		}
		if req.Platform != "ios" && req.Platform != "android" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "platform must be 'ios' or 'android'"})
			return
		}
		// SSRF guard: same check as dispatch handler.
		if isURL(req.DeviceToken) {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid device_token: must not be a URL"})
			return
		}

		appID := req.AppID
		if appID == "" {
			appID = "default"
		}

		device, err := UpsertDevice(ctx, pool, req.DeviceToken, req.Platform, appID, req.UserID)
		if err != nil {
			log.Printf("[push] upsert device: %v", err)
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to register device"})
			return
		}

		sdk.Respond(w, http.StatusOK, device)
	}
}

// --- Health handler ---

func handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sdk.Respond(w, http.StatusOK, map[string]string{"status": "ok", "plugin": "push"})
	}
}

// isURL reports whether s looks like an HTTP/HTTPS URL.
// Used as an SSRF guard for device tokens.
func isURL(s string) bool {
	if len(s) < 7 {
		return false
	}
	lower := s
	if len(lower) > 8 {
		lower = s[:8]
	}
	return lower[:4] == "http"
}
