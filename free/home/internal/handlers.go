package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/plugins/free/shared/httpclient"
)

// homeClient is the package-level HTTP client used for handler outbound
// calls. 30s budget per spec.
var homeClient = httpclient.New(httpclient.Options{Timeout: 30 * time.Second})

// Handlers holds the database pool and configuration for all HTTP handlers.
type Handlers struct {
	pool *pgxpool.Pool
	cfg  Config
}

// NewHandlers constructs a Handlers instance.
func NewHandlers(pool *pgxpool.Pool, cfg Config) *Handlers {
	return &Handlers{pool: pool, cfg: cfg}
}

// sa returns the source account ID from the request header, defaulting to "primary".
func (h *Handlers) sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-ID"); v != "" {
		return v
	}
	return "primary"
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// Health handles GET /health.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "plugin": "home"})
}

// Ready handles GET /ready — checks DB connectivity.
func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// HandleCommand handles POST /home/command — calls a Home Assistant service and logs it.
func (h *Handlers) HandleCommand(w http.ResponseWriter, r *http.Request) {
	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.EntityID == "" || req.Service == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "entity_id and service are required"})
		return
	}

	result, err := h.callHAService(r.Context(), req.EntityID, req.Service, req.Data)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	// Log the command
	sa := h.sa(r)
	_, _ = h.pool.Exec(r.Context(),
		`INSERT INTO np_home_command_log (source_account_id, entity_id, service, data, result) VALUES ($1, $2, $3, $4, $5)`,
		sa, req.EntityID, req.Service, req.Data, result,
	)

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}

// HandleListDevices handles GET /home/devices — returns all HA entity states.
func (h *Handlers) HandleListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.haGet(r.Context(), "/api/states")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": json.RawMessage(devices)})
}

// HandleGetState handles GET /home/state/{entity_id} — returns state of a specific entity.
func (h *Handlers) HandleGetState(w http.ResponseWriter, r *http.Request) {
	entityID := chi.URLParam(r, "entity_id")
	state, err := h.haGet(r.Context(), fmt.Sprintf("/api/states/%s", entityID))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(state)
}

// HandleActivateScene handles POST /home/scene/{scene_id} — activates a scene.
func (h *Handlers) HandleActivateScene(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "scene_id")
	entityID := fmt.Sprintf("scene.%s", sceneID)
	_, err := h.callHAService(r.Context(), entityID, "turn_on", nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleListAutomations handles GET /home/automations — returns HA automations.
func (h *Handlers) HandleListAutomations(w http.ResponseWriter, r *http.Request) {
	// Fetch all automation.* entities from HA states
	all, err := h.haGet(r.Context(), "/api/states")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	var states []json.RawMessage
	if err := json.Unmarshal(all, &states); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse HA response"})
		return
	}

	var automations []json.RawMessage
	for _, s := range states {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(s, &obj); err != nil {
			continue
		}
		var eid string
		if err := json.Unmarshal(obj["entity_id"], &eid); err != nil {
			continue
		}
		if strings.HasPrefix(eid, "automation.") {
			automations = append(automations, s)
		}
	}
	if automations == nil {
		automations = []json.RawMessage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"automations": automations})
}

// HandleToggleAutomation handles POST /home/automations/{id}/toggle — toggles an automation.
func (h *Handlers) HandleToggleAutomation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	entityID := fmt.Sprintf("automation.%s", id)
	_, err := h.callHAService(r.Context(), entityID, "toggle", nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// haGet performs a GET request against the Home Assistant REST API.
func (h *Handlers) haGet(ctx context.Context, path string) ([]byte, error) {
	url := strings.TrimRight(h.cfg.HAUrl, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if h.cfg.HAToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.cfg.HAToken)
	}
	resp, err := homeClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HA request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HA API returned %d", resp.StatusCode)
	}
	return body, nil
}

// callHAService calls a Home Assistant service (e.g. light.turn_on).
func (h *Handlers) callHAService(ctx context.Context, entityID, service string, data json.RawMessage) (string, error) {
	// Parse domain from entity_id: "light.living_room" → "light"
	parts := strings.SplitN(entityID, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid entity_id format: %s", entityID)
	}
	domain := parts[0]

	url := fmt.Sprintf("%s/api/services/%s/%s", strings.TrimRight(h.cfg.HAUrl, "/"), domain, service)

	// Merge data with entity_id
	payload := map[string]any{"entity_id": entityID}
	if len(data) > 0 && string(data) != "null" {
		var extra map[string]any
		if err := json.Unmarshal(data, &extra); err == nil {
			for k, v := range extra {
				payload[k] = v
			}
		}
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.cfg.HAToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.cfg.HAToken)
	}

	resp, err := homeClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HA service call failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		text, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HA service call failed: %d %s", resp.StatusCode, string(text))
	}
	return "ok", nil
}
